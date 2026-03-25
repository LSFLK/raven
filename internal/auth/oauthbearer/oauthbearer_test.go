package oauthbearer

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestParseInitialClientResponse(t *testing.T) {
	payload := "n,a=user@example.com,\x01auth=Bearer token-123\x01\x01"
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))

	token, authzid, err := ParseInitialClientResponse(encoded)
	if err != nil {
		t.Fatalf("ParseInitialClientResponse returned error: %v", err)
	}
	if token != "token-123" {
		t.Fatalf("expected token token-123, got %q", token)
	}
	if authzid != "user@example.com" {
		t.Fatalf("expected authzid user@example.com, got %q", authzid)
	}
}

func TestParseInitialClientResponseDetails(t *testing.T) {
	payload := "n,a=user@example.com,\x01user=alice\x01auth=Bearer token-abc\x01\x01"
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))

	token, authzid, user, err := ParseInitialClientResponseDetails(encoded)
	if err != nil {
		t.Fatalf("ParseInitialClientResponseDetails returned error: %v", err)
	}
	if token != "token-abc" {
		t.Fatalf("expected token token-abc, got %q", token)
	}
	if authzid != "user@example.com" {
		t.Fatalf("expected authzid user@example.com, got %q", authzid)
	}
	if user != "alice" {
		t.Fatalf("expected user alice, got %q", user)
	}
}

func TestParseRawInitialClientResponse_MissingBearer(t *testing.T) {
	_, _, err := ParseRawInitialClientResponse("n,,\x01k=v\x01\x01")
	if err == nil {
		t.Fatal("expected missing bearer error, got nil")
	}
}

func TestValidateAccessToken_Success(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate rsa key: %v", err)
	}
	jwkN, jwkE := rsaPublicJWK(t, &priv.PublicKey)

	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kty": "RSA",
				"kid": "kid-1",
				"n":   jwkN,
				"e":   jwkE,
			}},
		})
	}))
	defer jwksServer.Close()

	validator, err := NewValidator(Config{
		IssuerURL: "https://issuer.example.com",
		JWKSURL:   jwksServer.URL,
		Audiences: []string{"raven-imap"},
	})
	if err != nil {
		t.Fatalf("NewValidator failed: %v", err)
	}

	token, err := signToken(priv, "kid-1", jwt.MapClaims{
		"iss":                "https://issuer.example.com",
		"aud":                []string{"raven-imap"},
		"exp":                time.Now().Add(2 * time.Minute).Unix(),
		"email":              "alice@example.com",
		"preferred_username": "alice",
		"sub":                "sub-1",
	})
	if err != nil {
		t.Fatalf("signToken failed: %v", err)
	}

	claims, err := validator.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("ValidateAccessToken failed: %v", err)
	}
	if claims.Identity() != "alice@example.com" {
		t.Fatalf("expected identity alice@example.com, got %q", claims.Identity())
	}
}

func TestValidateAccessToken_Expired(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate rsa key: %v", err)
	}
	jwkN, jwkE := rsaPublicJWK(t, &priv.PublicKey)

	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kty": "RSA",
				"kid": "kid-1",
				"n":   jwkN,
				"e":   jwkE,
			}},
		})
	}))
	defer jwksServer.Close()

	validator, err := NewValidator(Config{
		IssuerURL: "https://issuer.example.com",
		JWKSURL:   jwksServer.URL,
		Audiences: []string{"raven-imap"},
		ClockSkew: 1 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewValidator failed: %v", err)
	}

	token, err := signToken(priv, "kid-1", jwt.MapClaims{
		"iss":   "https://issuer.example.com",
		"aud":   []string{"raven-imap"},
		"exp":   time.Now().Add(-2 * time.Minute).Unix(),
		"email": "alice@example.com",
	})
	if err != nil {
		t.Fatalf("signToken failed: %v", err)
	}

	_, err = validator.ValidateAccessToken(token)
	if err == nil {
		t.Fatal("expected expiration error, got nil")
	}
}

func TestValidateAccessToken_IssuerMismatch(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate rsa key: %v", err)
	}
	jwkN, jwkE := rsaPublicJWK(t, &priv.PublicKey)

	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kty": "RSA",
				"kid": "kid-1",
				"n":   jwkN,
				"e":   jwkE,
			}},
		})
	}))
	defer jwksServer.Close()

	validator, err := NewValidator(Config{
		IssuerURL: "https://issuer.example.com",
		JWKSURL:   jwksServer.URL,
	})
	if err != nil {
		t.Fatalf("NewValidator failed: %v", err)
	}

	token, err := signToken(priv, "kid-1", jwt.MapClaims{
		"iss":   "https://wrong-issuer.example.com",
		"exp":   time.Now().Add(2 * time.Minute).Unix(),
		"email": "alice@example.com",
	})
	if err != nil {
		t.Fatalf("signToken failed: %v", err)
	}

	_, err = validator.ValidateAccessToken(token)
	if err == nil || !strings.Contains(err.Error(), "issuer") {
		t.Fatalf("expected issuer mismatch error, got: %v", err)
	}
}

func TestValidateAccessToken_AudienceMismatch(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate rsa key: %v", err)
	}
	jwkN, jwkE := rsaPublicJWK(t, &priv.PublicKey)

	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kty": "RSA",
				"kid": "kid-1",
				"n":   jwkN,
				"e":   jwkE,
			}},
		})
	}))
	defer jwksServer.Close()

	validator, err := NewValidator(Config{
		JWKSURL:   jwksServer.URL,
		Audiences: []string{"raven-imap"},
	})
	if err != nil {
		t.Fatalf("NewValidator failed: %v", err)
	}

	token, err := signToken(priv, "kid-1", jwt.MapClaims{
		"aud":   []string{"other-aud"},
		"exp":   time.Now().Add(2 * time.Minute).Unix(),
		"email": "alice@example.com",
	})
	if err != nil {
		t.Fatalf("signToken failed: %v", err)
	}

	_, err = validator.ValidateAccessToken(token)
	if err == nil || !strings.Contains(err.Error(), "audience") {
		t.Fatalf("expected audience mismatch error, got: %v", err)
	}
}

func TestIdentityPrecedence(t *testing.T) {
	claims := Claims{Email: "first@example.com", PreferredUsername: "second", Subject: "third"}
	if claims.Identity() != "first@example.com" {
		t.Fatalf("expected email precedence, got %q", claims.Identity())
	}

	claims.Email = ""
	if claims.Identity() != "second" {
		t.Fatalf("expected preferred_username fallback, got %q", claims.Identity())
	}

	claims.PreferredUsername = ""
	claims.Username = "fourth"
	if claims.Identity() != "fourth" {
		t.Fatalf("expected username fallback, got %q", claims.Identity())
	}

	claims.Username = ""
	if claims.Identity() != "third" {
		t.Fatalf("expected sub fallback, got %q", claims.Identity())
	}
}

func signToken(priv *rsa.PrivateKey, kid string, claims jwt.MapClaims) (string, error) {
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	return tok.SignedString(priv)
}

func rsaPublicJWK(t *testing.T, pub *rsa.PublicKey) (string, string) {
	t.Helper()
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	eBytes := intToBytes(pub.E)
	e := base64.RawURLEncoding.EncodeToString(eBytes)
	return n, e
}

func intToBytes(v int) []byte {
	if v == 0 {
		return []byte{0}
	}
	buf := make([]byte, 0, 8)
	for v > 0 {
		buf = append([]byte{byte(v & 0xff)}, buf...)
		v >>= 8
	}
	return buf
}
