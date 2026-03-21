package thunder

import (
	"crypto/tls"
	"net/http"
	"time"
)

// GetHTTPClient returns an HTTP client with TLS verification disabled for self-signed certs
func GetHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}
