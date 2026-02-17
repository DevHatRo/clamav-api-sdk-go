package grpc

import (
	"time"

	grpclib "google.golang.org/grpc"
)

const (
	defaultTimeout        = 30 * time.Second
	defaultChunkSize      = 64 * 1024       // 64KB
	defaultMaxMessageSize = 200 * 1024 * 1024 // 200MB
)

// ClientOption configures the gRPC client.
type ClientOption func(*Client)

// WithDialOptions appends gRPC dial options to the connection.
func WithDialOptions(opts ...grpclib.DialOption) ClientOption {
	return func(c *Client) {
		c.dialOpts = append(c.dialOpts, opts...)
	}
}

// WithTimeout sets the default RPC timeout.
// If a context with a shorter deadline is provided to a method, that deadline takes precedence.
func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) {
		c.timeout = d
	}
}

// WithChunkSize sets the chunk size for streaming operations (default: 64KB).
func WithChunkSize(size int) ClientOption {
	return func(c *Client) {
		if size > 0 {
			c.chunkSize = size
		}
	}
}

// WithMaxMessageSize sets the max send/receive message size (default: 200MB).
func WithMaxMessageSize(size int) ClientOption {
	return func(c *Client) {
		if size > 0 {
			c.maxMessageSize = size
		}
	}
}
