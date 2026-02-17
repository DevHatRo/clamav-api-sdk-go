//go:build integration

package clamav

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func integrationRESTURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("CLAMAV_REST_URL")
	if url == "" {
		url = "http://localhost:6000"
	}
	return url
}

func integrationRESTClient(t *testing.T) *Client {
	t.Helper()
	client, err := NewClient(integrationRESTURL(t), WithTimeout(60*time.Second))
	if err != nil {
		t.Fatalf("failed to create REST client: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func TestIntegrationHealthCheck(t *testing.T) {
	client := integrationRESTClient(t)
	ctx := context.Background()

	result, err := client.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck error: %v", err)
	}
	if !result.Healthy {
		t.Errorf("expected healthy, got message: %s", result.Message)
	}
	t.Logf("Health: %v, Message: %s", result.Healthy, result.Message)
}

func TestIntegrationVersion(t *testing.T) {
	client := integrationRESTClient(t)
	ctx := context.Background()

	result, err := client.Version(ctx)
	if err != nil {
		t.Fatalf("Version error: %v", err)
	}
	if result.Version == "" {
		t.Error("expected non-empty version")
	}
	t.Logf("Version: %s, Commit: %s, Build: %s", result.Version, result.Commit, result.Build)
}

func TestIntegrationScanCleanFile(t *testing.T) {
	client := integrationRESTClient(t)
	ctx := context.Background()

	data := []byte("This is a clean test file with no malicious content.")
	result, err := client.ScanFile(ctx, data, "clean.txt")
	if err != nil {
		t.Fatalf("ScanFile error: %v", err)
	}
	if !result.IsClean() {
		t.Errorf("expected clean, got status %q message %q", result.Status, result.Message)
	}
	t.Logf("Scan result: status=%s, time=%.4fs", result.Status, result.ScanTime)
}

func TestIntegrationScanEicar(t *testing.T) {
	client := integrationRESTClient(t)
	ctx := context.Background()

	eicar := []byte(`X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*`)
	result, err := client.ScanFile(ctx, eicar, "eicar.txt")
	if err != nil {
		t.Fatalf("ScanFile error: %v", err)
	}
	if !result.IsInfected() {
		t.Errorf("expected infected, got status %q", result.Status)
	}
	if !strings.Contains(result.Message, "Eicar") {
		t.Errorf("expected Eicar in message, got %q", result.Message)
	}
	t.Logf("Scan result: status=%s, message=%s, time=%.4fs", result.Status, result.Message, result.ScanTime)
}

func TestIntegrationScanFilePath(t *testing.T) {
	client := integrationRESTClient(t)
	ctx := context.Background()

	result, err := client.ScanFilePath(ctx, "testdata/clean.txt")
	if err != nil {
		t.Fatalf("ScanFilePath error: %v", err)
	}
	if !result.IsClean() {
		t.Errorf("expected clean, got status %q", result.Status)
	}
}

func TestIntegrationScanReader(t *testing.T) {
	client := integrationRESTClient(t)
	ctx := context.Background()

	reader := strings.NewReader("another clean file content for reader test")
	result, err := client.ScanReader(ctx, reader, "reader-test.txt")
	if err != nil {
		t.Fatalf("ScanReader error: %v", err)
	}
	if !result.IsClean() {
		t.Errorf("expected clean, got status %q", result.Status)
	}
}

func TestIntegrationStreamScanClean(t *testing.T) {
	client := integrationRESTClient(t)
	ctx := context.Background()

	data := []byte("clean data for stream scanning")
	result, err := client.StreamScan(ctx, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("StreamScan error: %v", err)
	}
	if !result.IsClean() {
		t.Errorf("expected clean, got status %q", result.Status)
	}
	t.Logf("Stream scan result: status=%s, time=%.4fs", result.Status, result.ScanTime)
}

func TestIntegrationStreamScanEicar(t *testing.T) {
	client := integrationRESTClient(t)
	ctx := context.Background()

	eicar := []byte(`X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*`)
	result, err := client.StreamScan(ctx, bytes.NewReader(eicar), int64(len(eicar)))
	if err != nil {
		t.Fatalf("StreamScan error: %v", err)
	}
	if !result.IsInfected() {
		t.Errorf("expected infected, got status %q", result.Status)
	}
	t.Logf("Stream scan result: status=%s, message=%s", result.Status, result.Message)
}

func TestIntegrationStreamScanFile(t *testing.T) {
	client := integrationRESTClient(t)
	ctx := context.Background()

	result, err := client.StreamScanFile(ctx, "testdata/clean.txt")
	if err != nil {
		t.Fatalf("StreamScanFile error: %v", err)
	}
	if !result.IsClean() {
		t.Errorf("expected clean, got status %q", result.Status)
	}
}
