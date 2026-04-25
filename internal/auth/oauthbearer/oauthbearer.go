package oauthbearer

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/sync/singleflight"
)

const (
	defaultJWKSCacheTTL = 5 * time.Minute
	defaultClockSkew    = 60 * time.Second
)

type Config struct {
	IssuerURL string
	JWKSURL   string
	Audiences []string
	ClockSkew time.Duration
}

type Claims struct {
	Email              string
	PreferredUsername  string
	Username           string
	OrganizationUnitID string
	Subject            string
	Issuer             string
	Audience           []string
	ExpiresAt          time.Time
	GrantType          string
	ClientID           string
}

func (c Claims) Identity() string {
	if strings.TrimSpace(c.Email) != "" {
		return strings.TrimSpace(c.Email)
	}
	if strings.TrimSpace(c.PreferredUsername) != "" {
		return strings.TrimSpace(c.PreferredUsername)
	}
	if strings.TrimSpace(c.Username) != "" {
		return strings.TrimSpace(c.Username)
	}
	return strings.TrimSpace(c.Subject)
}

type Validator struct {
	cfg   Config
	mu    sync.RWMutex
	keys  map[string]*rsa.PublicKey
	expAt time.Time
	sf    singleflight.Group

	httpClient *http.Client
}

func NewValidator(cfg Config) (*Validator, error) {
	if strings.TrimSpace(cfg.JWKSURL) == "" {
		return nil, fmt.Errorf("oauth_jwks_url is required for OAUTHBEARER")
	}
	if cfg.ClockSkew <= 0 {
		cfg.ClockSkew = defaultClockSkew
	}
	return &Validator{
		cfg:        cfg,
		keys:       make(map[string]*rsa.PublicKey),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (v *Validator) ValidateAccessToken(token string) (Claims, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Claims{}, errors.New("empty access token")
	}

	parsed, err := jwt.Parse(token, v.keyFunc, jwt.WithValidMethods([]string{"RS256", "RS384", "RS512"}))
	if err != nil {
		return Claims{}, fmt.Errorf("invalid token signature: %w", err)
	}
	if !parsed.Valid {
		return Claims{}, errors.New("invalid token")
	}

	claimsMap, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return Claims{}, errors.New("invalid token claims")
	}

	if err := v.validateClaims(claimsMap); err != nil {
		return Claims{}, err
	}

	return extractClaims(claimsMap), nil
}

func (v *Validator) keyFunc(token *jwt.Token) (any, error) {
	alg := ""
	if token.Method != nil {
		alg = token.Method.Alg()
	}

	kidRaw, ok := token.Header["kid"]
	if !ok {
		log.Printf("OAUTHBEARER: key lookup failed alg=%q reason=missing_kid", alg)
		return nil, errors.New("missing kid header")
	}
	kid, ok := kidRaw.(string)
	if !ok || strings.TrimSpace(kid) == "" {
		log.Printf("OAUTHBEARER: key lookup failed alg=%q reason=invalid_kid kid_raw_type=%T", alg, kidRaw)
		return nil, errors.New("invalid kid header")
	}
	kid = strings.TrimSpace(kid)

	if key := v.getCachedKey(kid); key != nil {
		log.Printf("OAUTHBEARER: key lookup kid=%q alg=%q cache_hit=true", kid, alg)
		return key, nil
	}

	log.Printf("OAUTHBEARER: key lookup kid=%q alg=%q cache_hit=false refreshing_jwks=true", kid, alg)

	_, refreshErr, _ := v.sf.Do("jwks_refresh", func() (any, error) {
		return nil, v.refreshJWKS()
	})
	if refreshErr != nil {
		log.Printf("OAUTHBEARER: key lookup kid=%q alg=%q refresh_failed=%v", kid, alg, refreshErr)
		return nil, refreshErr
	}
	if key := v.getCachedKey(kid); key != nil {
		log.Printf("OAUTHBEARER: key lookup kid=%q alg=%q cache_hit=true source=post_refresh", kid, alg)
		return key, nil
	}

	log.Printf("OAUTHBEARER: key lookup kid=%q alg=%q result=not_found_after_refresh", kid, alg)

	return nil, fmt.Errorf("kid not found in jwks: %s", kid)
}

func (v *Validator) getCachedKey(kid string) *rsa.PublicKey {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if time.Now().After(v.expAt) {
		return nil
	}
	return v.keys[kid]
}

func (v *Validator) refreshJWKS() error {
	log.Printf("OAUTHBEARER: fetching jwks url=%q", v.cfg.JWKSURL)
	resp, err := v.httpClient.Get(v.cfg.JWKSURL) // #nosec G107 -- URL comes from trusted server config.
	if err != nil {
		return fmt.Errorf("failed to fetch jwks: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 2048))
		bodyPreview := ""
		if readErr != nil {
			bodyPreview = fmt.Sprintf("<failed to read response body: %v>", readErr)
		} else {
			bodyPreview = truncateForLog(string(bodyBytes), 300)
		}
		log.Printf("OAUTHBEARER: jwks request failed url=%q status=%d body=%q", v.cfg.JWKSURL, resp.StatusCode, bodyPreview)
		return fmt.Errorf("jwks request failed with status %d", resp.StatusCode)
	}

	bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if readErr != nil {
		return fmt.Errorf("failed to read jwks response body: %w", readErr)
	}
	log.Printf("OAUTHBEARER: jwks response ok url=%q status=%d body=%q", v.cfg.JWKSURL, resp.StatusCode, truncateForLog(string(bodyBytes), 500))

	var doc struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}

	if err := json.Unmarshal(bodyBytes, &doc); err != nil {
		return fmt.Errorf("failed to decode jwks: %w", err)
	}

	newKeys := make(map[string]*rsa.PublicKey)
	for _, key := range doc.Keys {
		if strings.ToUpper(strings.TrimSpace(key.Kty)) != "RSA" {
			continue
		}
		kid := strings.TrimSpace(key.Kid)
		if kid == "" {
			continue
		}

		pub, err := rsaPublicKeyFromJWK(key.N, key.E)
		if err != nil {
			continue
		}
		newKeys[kid] = pub
	}

	if len(newKeys) == 0 {
		log.Printf("OAUTHBEARER: jwks parse produced no usable RSA keys")
		return errors.New("no usable rsa keys found in jwks")
	}

	log.Printf("OAUTHBEARER: jwks refresh successful rsa_keys=%d", len(newKeys))

	v.mu.Lock()
	v.keys = newKeys
	v.expAt = time.Now().Add(defaultJWKSCacheTTL)
	v.mu.Unlock()

	return nil
}

func (v *Validator) validateClaims(claims jwt.MapClaims) error {
	now := time.Now()
	skew := v.cfg.ClockSkew

	expUnix, ok := claimAsInt64(claims, "exp")
	if !ok {
		log.Printf("OAUTHBEARER: claims validation failed reason=missing_exp")
		return errors.New("missing exp claim")
	}
	exp := time.Unix(expUnix, 0)
	if now.After(exp.Add(skew)) {
		log.Printf("OAUTHBEARER: claims validation failed reason=expired now=%s exp=%s skew=%s", now.UTC().Format(time.RFC3339), exp.UTC().Format(time.RFC3339), skew)
		return errors.New("token expired")
	}

	if nbfUnix, ok := claimAsInt64(claims, "nbf"); ok {
		nbf := time.Unix(nbfUnix, 0)
		if now.Add(skew).Before(nbf) {
			log.Printf("OAUTHBEARER: claims validation failed reason=not_yet_valid now=%s nbf=%s skew=%s", now.UTC().Format(time.RFC3339), nbf.UTC().Format(time.RFC3339), skew)
			return errors.New("token not valid yet")
		}
	}

	if iss := strings.TrimSpace(v.cfg.IssuerURL); iss != "" {
		issClaim, ok := claims["iss"].(string)
		if !ok || strings.TrimSpace(issClaim) != iss {
			log.Printf("OAUTHBEARER: claims validation failed reason=issuer_mismatch expected=%q actual=%q", iss, strings.TrimSpace(issClaim))
			return errors.New("issuer mismatch")
		}
	}

	if len(v.cfg.Audiences) > 0 {
		if !hasAcceptedAudience(claims["aud"], v.cfg.Audiences) {
			log.Printf("OAUTHBEARER: claims validation failed reason=audience_mismatch expected=%q actual=%q", strings.Join(v.cfg.Audiences, ","), strings.Join(audienceList(claims["aud"]), ","))
			return errors.New("audience mismatch")
		}
	}

	return nil
}

func extractClaims(claims jwt.MapClaims) Claims {
	result := Claims{}
	if email, ok := claims["email"].(string); ok {
		result.Email = strings.TrimSpace(email)
	}
	if preferred, ok := claims["preferred_username"].(string); ok {
		result.PreferredUsername = strings.TrimSpace(preferred)
	}
	if username, ok := claims["username"].(string); ok {
		result.Username = strings.TrimSpace(username)
	}
	if result.OrganizationUnitID == "" {
		result.OrganizationUnitID = claimAsString(claims, "ouId")
	}
	if result.OrganizationUnitID == "" {
		result.OrganizationUnitID = claimAsString(claims, "ouid")
	}
	if result.OrganizationUnitID == "" {
		result.OrganizationUnitID = claimAsString(claims, "ou_id")
	}
	if sub, ok := claims["sub"].(string); ok {
		result.Subject = strings.TrimSpace(sub)
	}
	if iss, ok := claims["iss"].(string); ok {
		result.Issuer = strings.TrimSpace(iss)
	}
	result.Audience = audienceList(claims["aud"])
	if expUnix, ok := claimAsInt64(claims, "exp"); ok {
		result.ExpiresAt = time.Unix(expUnix, 0)
	}
	if result.GrantType == "" {
		result.GrantType = claimAsString(claims, "grant_type")
	}
	if result.GrantType == "" {
		result.GrantType = claimAsString(claims, "grantTypes")
	}
	if result.GrantType == "" {
		result.GrantType = claimAsString(claims, "gty")
	}
	if result.ClientID == "" {
		result.ClientID = claimAsString(claims, "client_id")
	}
	if result.ClientID == "" {
		result.ClientID = claimAsString(claims, "clientId")
	}

	if strings.EqualFold(result.GrantType, "client_credentials") {
		log.Printf("OAUTHBEARER: extracted client_credentials claims grant_type=%q client_id=%q", result.GrantType, result.ClientID)
	}
	return result
}

func ParseInitialClientResponseDetails(encoded string) (token string, authzid string, user string, err error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return "", "", "", fmt.Errorf("invalid base64 OAUTHBEARER payload: %w", err)
	}
	return ParseRawInitialClientResponseDetails(string(decoded))
}

func ParseRawInitialClientResponseDetails(raw string) (token string, authzid string, user string, err error) {
	fields := strings.Split(raw, "\x01")
	if len(fields) == 0 {
		return "", "", "", errors.New("empty OAUTHBEARER payload")
	}

	user = ""

	if gs2 := strings.TrimSpace(fields[0]); gs2 != "" {
		authzid = extractAuthzidFromGS2(gs2)
	}

	for _, field := range fields {
		f := strings.TrimSpace(field)
		if len(f) == 0 {
			continue
		}
		if strings.HasPrefix(strings.ToLower(f), "user=") {
			user = strings.TrimSpace(f[len("user="):])
		}
		if strings.HasPrefix(strings.ToLower(f), "auth=bearer ") {
			token = strings.TrimSpace(f[len("auth=Bearer "):])
			if token == "" {
				return "", authzid, user, errors.New("empty bearer token")
			}
			log.Printf("OAUTHBEARER: parsed client response authzid=%q user=%q token_len=%d token_preview=%q", authzid, user, len(token), truncateForLog(token, 80))
			return token, authzid, user, nil
		}
	}

	return "", authzid, user, errors.New("missing auth=Bearer field")
}

func extractAuthzidFromGS2(gs2 string) string {
	idx := strings.Index(gs2, ",a=")
	if idx == -1 {
		return ""
	}
	rest := gs2[idx+3:]
	end := strings.Index(rest, ",")
	if end == -1 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:end])
}

func rsaPublicKeyFromJWK(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(nB64))
	if err != nil {
		return nil, fmt.Errorf("invalid jwk modulus: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(eB64))
	if err != nil {
		return nil, fmt.Errorf("invalid jwk exponent: %w", err)
	}
	if len(eBytes) == 0 {
		return nil, errors.New("empty jwk exponent")
	}

	e := 0
	for _, b := range eBytes {
		e = e<<8 + int(b)
	}
	if e <= 0 {
		return nil, errors.New("invalid jwk exponent value")
	}

	pub := &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: e,
	}
	if pub.N.Sign() <= 0 {
		return nil, errors.New("invalid jwk modulus value")
	}

	return pub, nil
}

func claimAsInt64(claims jwt.MapClaims, key string) (int64, bool) {
	v, ok := claims[key]
	if !ok {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case int64:
		return t, true
	case json.Number:
		i, err := t.Int64()
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

func claimAsString(claims jwt.MapClaims, key string) string {
	v, ok := claims[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func audienceList(raw any) []string {
	switch t := raw.(type) {
	case string:
		if strings.TrimSpace(t) == "" {
			return nil
		}
		return []string{strings.TrimSpace(t)}
	case []any:
		out := make([]string, 0, len(t))
		for _, v := range t {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		return nil
	}
}

func hasAcceptedAudience(raw any, accepted []string) bool {
	if len(accepted) == 0 {
		return true
	}
	provided := audienceList(raw)
	if len(provided) == 0 {
		return false
	}
	acceptedMap := make(map[string]struct{}, len(accepted))
	for _, a := range accepted {
		trimmed := strings.TrimSpace(a)
		if trimmed != "" {
			acceptedMap[trimmed] = struct{}{}
		}
	}
	for _, aud := range provided {
		if _, ok := acceptedMap[aud]; ok {
			return true
		}
	}
	return false
}

func truncateForLog(value string, maxLen int) string {
	if maxLen <= 0 || len(value) <= maxLen {
		return value
	}
	return value[:maxLen] + "..."
}
