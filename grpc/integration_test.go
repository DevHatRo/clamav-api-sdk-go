//go:build integration

package grpc

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	clamav "github.com/DevHatRo/clamav-api-sdk-go"
)

func integrationGRPCAddr(t *testing.T) string {
	t.Helper()
	addr := os.Getenv("CLAMAV_GRPC_ADDR")
	if addr == "" {
		addr = "localhost:9000"
	}
	return addr
}

func integrationGRPCClient(t *testing.T) *Client {
	t.Helper()
	client, err := NewClient(integrationGRPCAddr(t), WithTimeout(60*time.Second))
	if err != nil {
		t.Fatalf("failed to create gRPC client: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func TestIntegrationGRPCHealthCheck(t *testing.T) {
	client := integrationGRPCClient(t)
	ctx := context.Background()

	result, err := client.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck error: %v", err)
	}
	if !result.Healthy {
		t.Errorf("expected healthy, got message: %s", result.Message)
	}
	t.Logf("gRPC Health: %v, Message: %s", result.Healthy, result.Message)
}

func TestIntegrationGRPCScanFile(t *testing.T) {
	client := integrationGRPCClient(t)
	ctx := context.Background()

	t.Run("clean file", func(t *testing.T) {
		data := []byte("This is a clean test file.")
		result, err := client.ScanFile(ctx, data, "clean.txt")
		if err != nil {
			t.Fatalf("ScanFile error: %v", err)
		}
		if !result.IsClean() {
			t.Errorf("expected clean, got status %q message %q", result.Status, result.Message)
		}
		t.Logf("ScanFile result: status=%s, time=%.4fs", result.Status, result.ScanTime)
	})

	t.Run("eicar file", func(t *testing.T) {
		eicar := []byte(`X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*`)
		result, err := client.ScanFile(ctx, eicar, "eicar.txt")
		if err != nil {
			t.Fatalf("ScanFile error: %v", err)
		}
		if !result.IsInfected() {
			t.Errorf("expected infected, got status %q", result.Status)
		}
		if !strings.Contains(strings.ToLower(result.Message), "eicar") {
			t.Errorf("expected eicar (case-insensitive) in message, got %q", result.Message)
		}
		t.Logf("ScanFile result: status=%s, message=%s", result.Status, result.Message)
	})
}

func TestIntegrationGRPCScanStream(t *testing.T) {
	client := integrationGRPCClient(t)
	ctx := context.Background()

	t.Run("clean stream", func(t *testing.T) {
		data := []byte("clean data for gRPC stream scanning")
		result, err := client.ScanStream(ctx, data, "stream-clean.txt")
		if err != nil {
			t.Fatalf("ScanStream error: %v", err)
		}
		if !result.IsClean() {
			t.Errorf("expected clean, got status %q", result.Status)
		}
		t.Logf("ScanStream result: status=%s, time=%.4fs", result.Status, result.ScanTime)
	})

	t.Run("eicar stream", func(t *testing.T) {
		eicar := []byte(`X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*`)
		result, err := client.ScanStream(ctx, eicar, "stream-eicar.txt")
		if err != nil {
			t.Fatalf("ScanStream error: %v", err)
		}
		if !result.IsInfected() {
			t.Errorf("expected infected, got status %q", result.Status)
		}
		t.Logf("ScanStream result: status=%s, message=%s", result.Status, result.Message)
	})
}

func TestIntegrationGRPCScanStreamFile(t *testing.T) {
	client := integrationGRPCClient(t)
	ctx := context.Background()

	result, err := client.ScanStreamFile(ctx, "../testdata/clean.txt")
	if err != nil {
		t.Fatalf("ScanStreamFile error: %v", err)
	}
	if !result.IsClean() {
		t.Errorf("expected clean, got status %q", result.Status)
	}
}

func TestIntegrationGRPCScanMultiple(t *testing.T) {
	client := integrationGRPCClient(t)
	ctx := context.Background()

	eicar := []byte(`X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*`)
	files := []clamav.FileInput{
		{Data: []byte("clean content 1"), Filename: "clean1.txt"},
		{Data: eicar, Filename: "eicar.txt"},
		{Data: []byte("clean content 2"), Filename: "clean2.txt"},
	}

	results, err := client.ScanMultiple(ctx, files)
	if err != nil {
		t.Fatalf("ScanMultiple error: %v", err)
	}

	var cleanCount, infectedCount int
	for result := range results {
		t.Logf("ScanMultiple result: file=%s, status=%s, message=%s",
			result.Filename, result.Status, result.Message)

		switch {
		case result.IsClean():
			cleanCount++
		case result.IsInfected():
			infectedCount++
		default:
			t.Errorf("unexpected status %q for %s", result.Status, result.Filename)
		}
	}

	if cleanCount != 2 {
		t.Errorf("cleanCount = %d, want 2", cleanCount)
	}
	if infectedCount != 1 {
		t.Errorf("infectedCount = %d, want 1", infectedCount)
	}
}

func TestIntegrationGRPCScanMultipleCallback(t *testing.T) {
	client := integrationGRPCClient(t)
	ctx := context.Background()

	files := []clamav.FileInput{
		{Data: []byte("callback file 1"), Filename: "cb1.txt"},
		{Data: []byte("callback file 2"), Filename: "cb2.txt"},
	}

	var results []*clamav.ScanResult
	err := client.ScanMultipleCallback(ctx, files, func(r *clamav.ScanResult) {
		t.Logf("Callback: file=%s, status=%s", r.Filename, r.Status)
		results = append(results, r)
	})
	if err != nil {
		t.Fatalf("ScanMultipleCallback error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("got %d results, want 2", len(results))
	}
}
