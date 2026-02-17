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
func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) {
		c.timeout = d
	}
}

// WithHeaders sets default headers sent with every request.
// These can be used for authentication tokens, custom tracing headers, etc.
func WithHeaders(headers map[string]string) ClientOption {
	return func(c *Client) {
		c.headers = headers
	}
}
