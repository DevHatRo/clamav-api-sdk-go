package clamav

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultTimeout = 30 * time.Second

	pathHealthCheck = "/api/health-check"
	pathVersion     = "/api/version"
	pathScan        = "/api/scan"
	pathStreamScan  = "/api/stream-scan"
)

// Client is the REST client for the ClamAV API.
// It is safe for concurrent use from multiple goroutines.
type Client struct {
	baseURL    string
	httpClient *http.Client
	timeout    time.Duration
	headers    map[string]string
}

// NewClient creates a REST client for the ClamAV API.
// baseURL is the server base URL, e.g. "http://localhost:6000".
func NewClient(baseURL string, opts ...ClientOption) (*Client, error) {
	baseURL = strings.TrimRight(baseURL, "/")

	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("invalid base URL: %s", baseURL), err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, NewValidationError(fmt.Sprintf("base URL must include scheme and host: %s", baseURL), nil)
	}

	c := &Client{
		baseURL: baseURL,
		timeout: defaultTimeout,
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.httpClient == nil {
		c.httpClient = &http.Client{
			Timeout: c.timeout,
		}
	}

	return c, nil
}

// Close releases any resources held by the client.
func (c *Client) Close() error {
	c.httpClient.CloseIdleConnections()
	return nil
}

// HealthCheck checks if the ClamAV service is healthy.
func (c *Client) HealthCheck(ctx context.Context) (*HealthCheckResult, error) {
	req, err := c.newRequest(ctx, http.MethodGet, pathHealthCheck, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, NewServiceError("failed to decode health check response", resp.StatusCode, err)
	}

	return &HealthCheckResult{
		Healthy: resp.StatusCode == http.StatusOK && body.Message == "ok",
		Message: body.Message,
	}, nil
}

// Version returns the ClamAV API server version info.
func (c *Client) Version(ctx context.Context) (*VersionResult, error) {
	req, err := c.newRequest(ctx, http.MethodGet, pathVersion, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var result VersionResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, NewServiceError("failed to decode version response", resp.StatusCode, err)
	}

	return &result, nil
}

// ScanFile scans file data provided as a byte slice via multipart upload.
// filename is optional metadata sent with the multipart upload.
func (c *Client) ScanFile(ctx context.Context, data []byte, filename string) (*ScanResult, error) {
	return c.ScanReader(ctx, bytes.NewReader(data), filename)
}

// ScanFilePath reads a file from disk and scans it via multipart upload.
func (c *Client) ScanFilePath(ctx context.Context, filePath string) (*ScanResult, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to open file: %s", filePath), err)
	}
	defer f.Close()

	return c.ScanReader(ctx, f, filepath.Base(filePath))
}

// ScanReader scans data from an io.Reader via multipart upload.
func (c *Client) ScanReader(ctx context.Context, r io.Reader, filename string) (*ScanResult, error) {
	if filename == "" {
		filename = "file"
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, NewValidationError("failed to create multipart form", err)
	}

	if _, err := io.Copy(part, r); err != nil {
		return nil, NewValidationError("failed to write file data", err)
	}

	if err := writer.Close(); err != nil {
		return nil, NewValidationError("failed to close multipart writer", err)
	}

	req, err := c.newRequest(ctx, http.MethodPost, pathScan, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return c.doScan(req)
}

// StreamScan scans data from an io.Reader via the stream-scan endpoint.
// size is the Content-Length to set (required, must be > 0).
// For unknown sizes, buffer into bytes first and use ScanFile instead.
func (c *Client) StreamScan(ctx context.Context, r io.Reader, size int64) (*ScanResult, error) {
	if size <= 0 {
		return nil, NewValidationError("size must be greater than 0", nil)
	}

	req, err := c.newRequest(ctx, http.MethodPost, pathStreamScan, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = size

	return c.doScan(req)
}

// StreamScanFile reads a file from disk and scans via the stream-scan endpoint.
func (c *Client) StreamScanFile(ctx context.Context, filePath string) (*ScanResult, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to open file: %s", filePath), err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to stat file: %s", filePath), err)
	}

	return c.StreamScan(ctx, f, stat.Size())
}

// newRequest creates an HTTP request with context, base URL, and default headers.
func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, NewConnectionError("failed to create request", err)
	}

	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	return req, nil
}

// do executes an HTTP request and maps transport errors to SDK error types.
func (c *Client) do(req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, c.classifyTransportError(err)
	}
	return resp, nil
}

// doScan executes a scan request and parses the response.
func (c *Client) doScan(req *http.Request) (*ScanResult, error) {
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var result ScanResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, NewServiceError("failed to decode scan response", resp.StatusCode, err)
	}

	return &result, nil
}

// handleErrorResponse maps HTTP error responses to SDK error types.
func (c *Client) handleErrorResponse(resp *http.Response) error {
	var body struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return NewServiceError(
			fmt.Sprintf("unexpected status %d and failed to decode error response", resp.StatusCode),
			resp.StatusCode, err,
		)
	}

	msg := body.Message
	if msg == "" {
		msg = body.Status
	}

	switch resp.StatusCode {
	case http.StatusBadRequest: // 400
		return NewValidationError(msg, nil)
	case 413: // Request Entity Too Large
		return NewValidationError(msg, nil)
	case 499: // Client closed request
		return NewTimeoutError(msg, nil)
	case http.StatusBadGateway: // 502
		return NewServiceError(msg, resp.StatusCode, nil)
	case http.StatusGatewayTimeout: // 504
		return NewTimeoutError(msg, nil)
	default:
		return NewServiceError(
			fmt.Sprintf("unexpected status %d: %s", resp.StatusCode, msg),
			resp.StatusCode, nil,
		)
	}
}

// classifyTransportError maps Go transport errors to SDK error types.
func (c *Client) classifyTransportError(err error) error {
	if err == nil {
		return nil
	}

	// Context cancellation or deadline exceeded
	if ctx := context.Canceled; err == ctx {
		return NewTimeoutError("request canceled", err)
	}
	if ctx := context.DeadlineExceeded; err == ctx {
		return NewTimeoutError("request timed out", err)
	}

	// Unwrap url.Error to check inner cause
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() {
			return NewTimeoutError("request timed out", err)
		}
		if errors.Is(urlErr.Err, context.Canceled) {
			return NewTimeoutError("request canceled", err)
		}
		if errors.Is(urlErr.Err, context.DeadlineExceeded) {
			return NewTimeoutError("request timed out", err)
		}
	}

	// Network errors (connection refused, DNS, etc.)
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return NewConnectionError("connection failed", err)
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return NewConnectionError("DNS resolution failed", err)
	}

	return NewConnectionError("request failed", err)
}
