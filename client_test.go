package clamav

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DevHatRo/clamav-api-sdk-go/internal/testutil"
)

// --- NewClient tests ---

func TestNewClient(t *testing.T) {
	t.Run("valid URL", func(t *testing.T) {
		client, err := NewClient("http://localhost:6000")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if client == nil {
			t.Fatal("client should not be nil")
		}
		client.Close()
	})

	t.Run("trailing slash stripped", func(t *testing.T) {
		client, err := NewClient("http://localhost:6000/")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if client.baseURL != "http://localhost:6000" {
			t.Errorf("baseURL = %q, want %q", client.baseURL, "http://localhost:6000")
		}
		client.Close()
	})

	t.Run("missing scheme", func(t *testing.T) {
		_, err := NewClient("localhost:6000")
		if err == nil {
			t.Fatal("expected error for missing scheme")
		}
		if !IsValidationError(err) {
			t.Errorf("expected validation error, got %T: %v", err, err)
		}
	})

	t.Run("empty URL", func(t *testing.T) {
		_, err := NewClient("")
		if err == nil {
			t.Fatal("expected error for empty URL")
		}
	})

	t.Run("with custom HTTP client", func(t *testing.T) {
		hc := &http.Client{Timeout: 5 * time.Second}
		client, err := NewClient("http://localhost:6000", WithHTTPClient(hc))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if client.httpClient != hc {
			t.Error("custom HTTP client not set")
		}
		client.Close()
	})

	t.Run("with timeout", func(t *testing.T) {
		client, err := NewClient("http://localhost:6000", WithTimeout(10*time.Second))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if client.timeout != 10*time.Second {
			t.Errorf("timeout = %v, want %v", client.timeout, 10*time.Second)
		}
		client.Close()
	})

	t.Run("with headers", func(t *testing.T) {
		headers := map[string]string{"X-Custom": "value"}
		client, err := NewClient("http://localhost:6000", WithHeaders(headers))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if client.headers["X-Custom"] != "value" {
			t.Error("custom headers not set")
		}
		client.Close()
	})
}

// --- HealthCheck tests ---

func TestHealthCheck(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		srv := testutil.NewMockServer(map[string]http.HandlerFunc{
			"/api/health-check": testutil.JSONHandler(http.StatusOK, map[string]string{"message": "ok"}),
		})
		defer srv.Close()

		client, _ := NewClient(srv.URL)
		defer client.Close()

		result, err := client.HealthCheck(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Healthy {
			t.Error("expected Healthy to be true")
		}
		if result.Message != "ok" {
			t.Errorf("Message = %q, want %q", result.Message, "ok")
		}
	})

	t.Run("unhealthy", func(t *testing.T) {
		srv := testutil.NewMockServer(map[string]http.HandlerFunc{
			"/api/health-check": testutil.JSONHandler(http.StatusBadGateway, map[string]string{"message": "Clamd service unavailable"}),
		})
		defer srv.Close()

		client, _ := NewClient(srv.URL)
		defer client.Close()

		result, err := client.HealthCheck(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Healthy {
			t.Error("expected Healthy to be false")
		}
		if result.Message != "Clamd service unavailable" {
			t.Errorf("Message = %q, want %q", result.Message, "Clamd service unavailable")
		}
	})

	t.Run("connection error", func(t *testing.T) {
		client, _ := NewClient("http://127.0.0.1:1")
		defer client.Close()

		_, err := client.HealthCheck(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsConnectionError(err) {
			t.Errorf("expected connection error, got: %v", err)
		}
	})

	t.Run("context canceled", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(2 * time.Second)
		}))
		defer srv.Close()

		client, _ := NewClient(srv.URL)
		defer client.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := client.HealthCheck(ctx)
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsTimeoutError(err) {
			t.Errorf("expected timeout error, got: %T %v", err, err)
		}
	})
}

// --- Version tests ---

func TestVersion(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := testutil.NewMockServer(map[string]http.HandlerFunc{
			"/api/version": testutil.JSONHandler(http.StatusOK, map[string]string{
				"version": "1.3.0",
				"commit":  "abc1234",
				"build":   "2025-10-16T12:00:00Z",
			}),
		})
		defer srv.Close()

		client, _ := NewClient(srv.URL)
		defer client.Close()

		result, err := client.Version(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Version != "1.3.0" {
			t.Errorf("Version = %q, want %q", result.Version, "1.3.0")
		}
		if result.Commit != "abc1234" {
			t.Errorf("Commit = %q, want %q", result.Commit, "abc1234")
		}
		if result.Build != "2025-10-16T12:00:00Z" {
			t.Errorf("Build = %q, want %q", result.Build, "2025-10-16T12:00:00Z")
		}
	})

	t.Run("server error", func(t *testing.T) {
		srv := testutil.NewMockServer(map[string]http.HandlerFunc{
			"/api/version": testutil.JSONHandler(http.StatusBadGateway, map[string]string{
				"message": "Clamd service unavailable",
			}),
		})
		defer srv.Close()

		client, _ := NewClient(srv.URL)
		defer client.Close()

		_, err := client.Version(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsServiceError(err) {
			t.Errorf("expected service error, got: %v", err)
		}
	})
}

// --- ScanFile tests ---

func TestScanFile(t *testing.T) {
	t.Run("clean file", func(t *testing.T) {
		srv := testutil.NewMockServer(map[string]http.HandlerFunc{
			"/api/scan": testutil.ScanHandler(func(data []byte, filename string) (int, interface{}) {
				return http.StatusOK, testutil.CleanScanResponse()
			}),
		})
		defer srv.Close()

		client, _ := NewClient(srv.URL)
		defer client.Close()

		result, err := client.ScanFile(context.Background(), []byte("clean content"), "clean.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsClean() {
			t.Errorf("expected clean, got status %q", result.Status)
		}
		if result.IsInfected() {
			t.Error("IsInfected should be false")
		}
	})

	t.Run("infected file", func(t *testing.T) {
		srv := testutil.NewMockServer(map[string]http.HandlerFunc{
			"/api/scan": testutil.ScanHandler(func(data []byte, filename string) (int, interface{}) {
				return http.StatusOK, testutil.InfectedScanResponse()
			}),
		})
		defer srv.Close()

		client, _ := NewClient(srv.URL)
		defer client.Close()

		result, err := client.ScanFile(context.Background(), []byte("X5O!P%@AP"), "eicar.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsInfected() {
			t.Errorf("expected infected, got status %q", result.Status)
		}
		if result.Message != "Eicar-Test-Signature" {
			t.Errorf("Message = %q, want %q", result.Message, "Eicar-Test-Signature")
		}
	})

	t.Run("error 400", func(t *testing.T) {
		srv := testutil.NewMockServer(map[string]http.HandlerFunc{
			"/api/scan": testutil.JSONHandler(http.StatusBadRequest, map[string]string{
				"message": "Provide a single file",
			}),
		})
		defer srv.Close()

		client, _ := NewClient(srv.URL)
		defer client.Close()

		_, err := client.ScanFile(context.Background(), []byte("data"), "test.txt")
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsValidationError(err) {
			t.Errorf("expected validation error, got: %v", err)
		}
	})

	t.Run("error 413", func(t *testing.T) {
		srv := testutil.NewMockServer(map[string]http.HandlerFunc{
			"/api/scan": testutil.JSONHandler(413, map[string]string{
				"message": "File too large. Maximum size is 209715200 bytes",
			}),
		})
		defer srv.Close()

		client, _ := NewClient(srv.URL)
		defer client.Close()

		_, err := client.ScanFile(context.Background(), []byte("data"), "test.txt")
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsValidationError(err) {
			t.Errorf("expected validation error, got: %v", err)
		}
	})

	t.Run("error 502", func(t *testing.T) {
		srv := testutil.NewMockServer(map[string]http.HandlerFunc{
			"/api/scan": testutil.JSONHandler(http.StatusBadGateway, map[string]string{
				"status":  "Clamd service down",
				"message": "Scanning service unavailable",
			}),
		})
		defer srv.Close()

		client, _ := NewClient(srv.URL)
		defer client.Close()

		_, err := client.ScanFile(context.Background(), []byte("data"), "test.txt")
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsServiceError(err) {
			t.Errorf("expected service error, got: %v", err)
		}
	})

	t.Run("error 504", func(t *testing.T) {
		srv := testutil.NewMockServer(map[string]http.HandlerFunc{
			"/api/scan": testutil.JSONHandler(http.StatusGatewayTimeout, map[string]string{
				"status":  "Scan timeout",
				"message": "scan operation timed out after 300 seconds",
			}),
		})
		defer srv.Close()

		client, _ := NewClient(srv.URL)
		defer client.Close()

		_, err := client.ScanFile(context.Background(), []byte("data"), "test.txt")
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsTimeoutError(err) {
			t.Errorf("expected timeout error, got: %v", err)
		}
	})

	t.Run("default filename", func(t *testing.T) {
		var receivedFilename string
		srv := testutil.NewMockServer(map[string]http.HandlerFunc{
			"/api/scan": testutil.ScanHandler(func(data []byte, filename string) (int, interface{}) {
				receivedFilename = filename
				return http.StatusOK, testutil.CleanScanResponse()
			}),
		})
		defer srv.Close()

		client, _ := NewClient(srv.URL)
		defer client.Close()

		_, err := client.ScanFile(context.Background(), []byte("data"), "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if receivedFilename != "file" {
			t.Errorf("filename = %q, want %q", receivedFilename, "file")
		}
	})
}

// --- ScanFilePath tests ---

func TestScanFilePath(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := testutil.NewMockServer(map[string]http.HandlerFunc{
			"/api/scan": testutil.ScanHandler(func(data []byte, filename string) (int, interface{}) {
				return http.StatusOK, testutil.CleanScanResponse()
			}),
		})
		defer srv.Close()

		client, _ := NewClient(srv.URL)
		defer client.Close()

		result, err := client.ScanFilePath(context.Background(), "testdata/clean.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsClean() {
			t.Errorf("expected clean, got status %q", result.Status)
		}
	})

	t.Run("file not found", func(t *testing.T) {
		client, _ := NewClient("http://localhost:6000")
		defer client.Close()

		_, err := client.ScanFilePath(context.Background(), "testdata/nonexistent.txt")
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsValidationError(err) {
			t.Errorf("expected validation error, got: %v", err)
		}
	})
}

// --- ScanReader tests ---

func TestScanReader(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := testutil.NewMockServer(map[string]http.HandlerFunc{
			"/api/scan": testutil.ScanHandler(func(data []byte, filename string) (int, interface{}) {
				return http.StatusOK, testutil.CleanScanResponse()
			}),
		})
		defer srv.Close()

		client, _ := NewClient(srv.URL)
		defer client.Close()

		reader := strings.NewReader("some file content")
		result, err := client.ScanReader(context.Background(), reader, "test.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsClean() {
			t.Errorf("expected clean, got status %q", result.Status)
		}
	})

	t.Run("filename sent in multipart", func(t *testing.T) {
		var receivedFilename string
		srv := testutil.NewMockServer(map[string]http.HandlerFunc{
			"/api/scan": testutil.ScanHandler(func(data []byte, filename string) (int, interface{}) {
				receivedFilename = filename
				return http.StatusOK, testutil.CleanScanResponse()
			}),
		})
		defer srv.Close()

		client, _ := NewClient(srv.URL)
		defer client.Close()

		_, err := client.ScanReader(context.Background(), strings.NewReader("data"), "myfile.pdf")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if receivedFilename != "myfile.pdf" {
			t.Errorf("filename = %q, want %q", receivedFilename, "myfile.pdf")
		}
	})
}

// --- StreamScan tests ---

func TestStreamScan(t *testing.T) {
	t.Run("clean", func(t *testing.T) {
		srv := testutil.NewMockServer(map[string]http.HandlerFunc{
			"/api/stream-scan": testutil.ScanHandler(func(data []byte, filename string) (int, interface{}) {
				return http.StatusOK, testutil.CleanScanResponse()
			}),
		})
		defer srv.Close()

		client, _ := NewClient(srv.URL)
		defer client.Close()

		data := []byte("clean content")
		result, err := client.StreamScan(context.Background(), bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsClean() {
			t.Errorf("expected clean, got status %q", result.Status)
		}
	})

	t.Run("infected", func(t *testing.T) {
		srv := testutil.NewMockServer(map[string]http.HandlerFunc{
			"/api/stream-scan": testutil.ScanHandler(func(data []byte, filename string) (int, interface{}) {
				return http.StatusOK, testutil.InfectedScanResponse()
			}),
		})
		defer srv.Close()

		client, _ := NewClient(srv.URL)
		defer client.Close()

		data := []byte("eicar data")
		result, err := client.StreamScan(context.Background(), bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsInfected() {
			t.Errorf("expected infected, got status %q", result.Status)
		}
	})

	t.Run("zero size", func(t *testing.T) {
		client, _ := NewClient("http://localhost:6000")
		defer client.Close()

		_, err := client.StreamScan(context.Background(), strings.NewReader(""), 0)
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsValidationError(err) {
			t.Errorf("expected validation error, got: %v", err)
		}
	})

	t.Run("negative size", func(t *testing.T) {
		client, _ := NewClient("http://localhost:6000")
		defer client.Close()

		_, err := client.StreamScan(context.Background(), strings.NewReader("data"), -1)
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsValidationError(err) {
			t.Errorf("expected validation error, got: %v", err)
		}
	})

	t.Run("error 400", func(t *testing.T) {
		srv := testutil.NewMockServer(map[string]http.HandlerFunc{
			"/api/stream-scan": testutil.JSONHandler(http.StatusBadRequest, map[string]string{
				"message": "Content-Length header is required and must be greater than 0",
			}),
		})
		defer srv.Close()

		client, _ := NewClient(srv.URL)
		defer client.Close()

		_, err := client.StreamScan(context.Background(), strings.NewReader("data"), 4)
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsValidationError(err) {
			t.Errorf("expected validation error, got: %v", err)
		}
	})
}

// --- StreamScanFile tests ---

func TestStreamScanFile(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := testutil.NewMockServer(map[string]http.HandlerFunc{
			"/api/stream-scan": testutil.ScanHandler(func(data []byte, filename string) (int, interface{}) {
				return http.StatusOK, testutil.CleanScanResponse()
			}),
		})
		defer srv.Close()

		client, _ := NewClient(srv.URL)
		defer client.Close()

		result, err := client.StreamScanFile(context.Background(), "testdata/clean.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsClean() {
			t.Errorf("expected clean, got status %q", result.Status)
		}
	})

	t.Run("file not found", func(t *testing.T) {
		client, _ := NewClient("http://localhost:6000")
		defer client.Close()

		_, err := client.StreamScanFile(context.Background(), "testdata/nonexistent.txt")
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsValidationError(err) {
			t.Errorf("expected validation error, got: %v", err)
		}
	})
}

// --- Custom headers test ---

func TestCustomHeaders(t *testing.T) {
	var receivedHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("X-API-Key")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "ok"}) //nolint:errcheck
	}))
	defer srv.Close()

	client, _ := NewClient(srv.URL, WithHeaders(map[string]string{
		"X-API-Key": "secret-token",
	}))
	defer client.Close()

	_, err := client.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedHeader != "secret-token" {
		t.Errorf("header = %q, want %q", receivedHeader, "secret-token")
	}
}

// --- Context timeout test ---

func TestContextTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer srv.Close()

	client, _ := NewClient(srv.URL, WithHTTPClient(&http.Client{Timeout: 0}))
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.ScanFile(ctx, []byte("data"), "test.txt")
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsTimeoutError(err) {
		t.Errorf("expected timeout error, got: %T %v", err, err)
	}
}

// --- ScanResult method tests ---

func TestScanResultMethods(t *testing.T) {
	clean := &ScanResult{Status: "OK"}
	if !clean.IsClean() {
		t.Error("IsClean should be true for OK status")
	}
	if clean.IsInfected() {
		t.Error("IsInfected should be false for OK status")
	}

	infected := &ScanResult{Status: "FOUND", Message: "Virus"}
	if infected.IsClean() {
		t.Error("IsClean should be false for FOUND status")
	}
	if !infected.IsInfected() {
		t.Error("IsInfected should be true for FOUND status")
	}

	errResult := &ScanResult{Status: "ERROR"}
	if errResult.IsClean() {
		t.Error("IsClean should be false for ERROR status")
	}
	if errResult.IsInfected() {
		t.Error("IsInfected should be false for ERROR status")
	}
}

// --- Concurrent usage test ---

func TestConcurrentUsage(t *testing.T) {
	srv := testutil.NewMockServer(map[string]http.HandlerFunc{
		"/api/scan": testutil.ScanHandler(func(data []byte, filename string) (int, interface{}) {
			return http.StatusOK, testutil.CleanScanResponse()
		}),
		"/api/health-check": testutil.JSONHandler(http.StatusOK, map[string]string{"message": "ok"}),
	})
	defer srv.Close()

	client, _ := NewClient(srv.URL)
	defer client.Close()

	ctx := context.Background()
	errs := make(chan error, 20)

	for i := 0; i < 10; i++ {
		go func() {
			_, err := client.ScanFile(ctx, []byte("data"), "test.txt")
			errs <- err
		}()
		go func() {
			_, err := client.HealthCheck(ctx)
			errs <- err
		}()
	}

	for i := 0; i < 20; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent request failed: %v", err)
		}
	}
}

// --- Close test ---

func TestClose(t *testing.T) {
	client, _ := NewClient("http://localhost:6000")
	err := client.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

// --- ScanFilePath verifies actual file content is sent ---

func TestScanFilePathContent(t *testing.T) {
	// Create a temp file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("specific test content for verification")
	if err := os.WriteFile(tmpFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	var receivedData []byte
	var receivedFilename string
	srv := testutil.NewMockServer(map[string]http.HandlerFunc{
		"/api/scan": testutil.ScanHandler(func(data []byte, filename string) (int, interface{}) {
			receivedData = data
			receivedFilename = filename
			return http.StatusOK, testutil.CleanScanResponse()
		}),
	})
	defer srv.Close()

	client, _ := NewClient(srv.URL)
	defer client.Close()

	result, err := client.ScanFilePath(context.Background(), tmpFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsClean() {
		t.Errorf("expected clean result")
	}
	if !bytes.Equal(receivedData, content) {
		t.Errorf("received data = %q, want %q", receivedData, content)
	}
	if receivedFilename != "test.txt" {
		t.Errorf("filename = %q, want %q", receivedFilename, "test.txt")
	}
}

// --- Error response with unexpected status code ---

func TestUnexpectedStatusCode(t *testing.T) {
	srv := testutil.NewMockServer(map[string]http.HandlerFunc{
		"/api/scan": testutil.JSONHandler(http.StatusInternalServerError, map[string]string{
			"message": "unexpected internal error",
		}),
	})
	defer srv.Close()

	client, _ := NewClient(srv.URL)
	defer client.Close()

	_, err := client.ScanFile(context.Background(), []byte("data"), "test.txt")
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsServiceError(err) {
		t.Errorf("expected service error for unexpected status, got: %v", err)
	}
}

// --- Malformed JSON response test ---

func TestMalformedJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "not json") //nolint:errcheck
	}))
	defer srv.Close()

	client, _ := NewClient(srv.URL)
	defer client.Close()

	_, err := client.Version(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}
