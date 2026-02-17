package clamav

import (
	"net/http"
	"time"
)

// ClientOption configures the REST client.
type ClientOption func(*Client)

// WithHTTPClient sets a custom *http.Client for the REST client.
// This allows full control over transport, TLS, timeouts, etc.
func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = hc
	}
}

// WithTimeout sets the default request timeout for all operations.
// If a context with a shorter deadline is provided to a method, that deadline takes precedence.
// Non-positive durations are ignored (no-op).
func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) {
		if d > 0 {
			c.timeout = d
		}
	}
}

// WithHeaders sets default headers sent with every request.
// These can be used for authentication tokens, custom tracing headers, etc.
// A defensive copy of the map is stored so the client is not affected by later mutations.
func WithHeaders(headers map[string]string) ClientOption {
	return func(c *Client) {
		if headers == nil {
			c.headers = nil
			return
		}
		c.headers = make(map[string]string, len(headers))
		for k, v := range headers {
			c.headers[k] = v
		}
	}
}
