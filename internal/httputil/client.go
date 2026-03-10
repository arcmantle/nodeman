package httputil

import (
	"net/http"
	"time"
)

// NewClient creates an HTTP client with the given timeout that respects
// standard proxy environment variables (HTTP_PROXY, HTTPS_PROXY, NO_PROXY).
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{Proxy: http.ProxyFromEnvironment},
	}
}
