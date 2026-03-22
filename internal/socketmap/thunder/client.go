package thunder

import (
	"crypto/tls"
	"net/http"
	"time"
)

// GetHTTPClient returns an HTTP client with secure TLS defaults.
func GetHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}
}
