package grpc

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clamav "github.com/DevHatRo/clamav-api-sdk-go"
	pb "github.com/DevHatRo/clamav-api-sdk-go/grpc/proto"
	grpclib "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

// --- Mock gRPC server ---

type mockClamAVServer struct {
	pb.UnimplementedClamAVScannerServer
	healthStatus  string
	healthMessage string
	scanFunc      func(data []byte, filename string) (*pb.ScanResponse, error)
}

func (s *mockClamAVServer) HealthCheck(_ context.Context, _ *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	st := s.healthStatus
	if st == "" {
		st = "healthy"
	}
	return &pb.HealthCheckResponse{
		Status:  st,
		Message: s.healthMessage,
	}, nil
}

func (s *mockClamAVServer) ScanFile(_ context.Context, req *pb.ScanFileRequest) (*pb.ScanResponse, error) {
	if len(req.Data) == 0 {
		return nil, status.Error(codes.InvalidArgument, "file data is required")
	}
	if s.scanFunc != nil {
		return s.scanFunc(req.Data, req.Filename)
	}
	return &pb.ScanResponse{
		Status:   "OK",
		Message:  "",
		ScanTime: 0.001,
		Filename: req.Filename,
	}, nil
}

func (s *mockClamAVServer) ScanStream(stream pb.ClamAVScanner_ScanStreamServer) error {
	var allData []byte
	var filename string

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if filename == "" && req.Filename != "" {
			filename = req.Filename
		}
		allData = append(allData, req.Chunk...)
		if req.IsLast {
			break
		}
	}

	if s.scanFunc != nil {
		resp, err := s.scanFunc(allData, filename)
		if err != nil {
			return err
		}
		return stream.SendAndClose(resp)
	}

	return stream.SendAndClose(&pb.ScanResponse{
		Status:   "OK",
		Message:  "",
		ScanTime: 0.001,
		Filename: filename,
	})
}

func (s *mockClamAVServer) ScanMultiple(stream pb.ClamAVScanner_ScanMultipleServer) error {
	var currentData []byte
	var currentFilename string

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		if currentFilename == "" && req.Filename != "" {
			currentFilename = req.Filename
		}
		currentData = append(currentData, req.Chunk...)

		if req.IsLast {
			var resp *pb.ScanResponse
			if s.scanFunc != nil {
				var scanErr error
				resp, scanErr = s.scanFunc(currentData, currentFilename)
				if scanErr != nil {
					resp = &pb.ScanResponse{
						Status:   "ERROR",
						Message:  scanErr.Error(),
						Filename: currentFilename,
					}
				}
			} else {
				resp = &pb.ScanResponse{
					Status:   "OK",
					Message:  "",
					ScanTime: 0.001,
					Filename: currentFilename,
				}
			}
			if err := stream.Send(resp); err != nil {
				return err
			}
			currentData = nil
			currentFilename = ""
		}
	}
}

// --- Test environment helpers ---

type testEnv struct {
	mock     *mockClamAVServer
	client   *Client
	lis      *bufconn.Listener
	grpcSrv  *grpclib.Server
	grpcConn *grpclib.ClientConn
}

func newTestEnv(t *testing.T, mock *mockClamAVServer) *testEnv {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	srv := grpclib.NewServer()
	pb.RegisterClamAVScannerServer(srv, mock)

	go func() {
		srv.Serve(lis) //nolint:errcheck
	}()

	conn, err := grpclib.NewClient("passthrough:///bufconn",
		grpclib.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpclib.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to create bufconn client: %v", err)
	}

	client := &Client{
		conn:           conn,
		scanner:        pb.NewClamAVScannerClient(conn),
		timeout:        5 * time.Second,
		chunkSize:      defaultChunkSize,
		maxMessageSize: defaultMaxMessageSize,
	}

	return &testEnv{
		mock:     mock,
		client:   client,
		lis:      lis,
		grpcSrv:  srv,
		grpcConn: conn,
	}
}

func (e *testEnv) close() {
	e.grpcSrv.GracefulStop()
	e.grpcConn.Close()
	e.lis.Close()
}

// --- HealthCheck tests ---

func TestHealthCheck(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		env := newTestEnv(t, &mockClamAVServer{
			healthStatus:  "healthy",
			healthMessage: "ClamAV is running",
		})
		defer env.close()

		result, err := env.client.HealthCheck(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Healthy {
			t.Error("expected Healthy to be true")
		}
		if result.Message != "ClamAV is running" {
			t.Errorf("Message = %q, want %q", result.Message, "ClamAV is running")
		}
	})

	t.Run("unhealthy", func(t *testing.T) {
		env := newTestEnv(t, &mockClamAVServer{
			healthStatus:  "unhealthy",
			healthMessage: "ClamAV is down",
		})
		defer env.close()

		result, err := env.client.HealthCheck(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Healthy {
			t.Error("expected Healthy to be false")
		}
	})
}

// --- ScanFile tests ---

func TestScanFile(t *testing.T) {
	t.Run("clean file", func(t *testing.T) {
		env := newTestEnv(t, &mockClamAVServer{})
		defer env.close()

		result, err := env.client.ScanFile(context.Background(), []byte("clean data"), "clean.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsClean() {
			t.Errorf("expected clean, got status %q", result.Status)
		}
		if result.Filename != "clean.txt" {
			t.Errorf("Filename = %q, want %q", result.Filename, "clean.txt")
		}
	})

	t.Run("infected file", func(t *testing.T) {
		env := newTestEnv(t, &mockClamAVServer{
			scanFunc: func(data []byte, filename string) (*pb.ScanResponse, error) {
				if strings.Contains(string(data), "EICAR") {
					return &pb.ScanResponse{
						Status:   "FOUND",
						Message:  "Eicar-Test-Signature",
						ScanTime: 0.002,
						Filename: filename,
					}, nil
				}
				return &pb.ScanResponse{Status: "OK", Filename: filename}, nil
			},
		})
		defer env.close()

		result, err := env.client.ScanFile(context.Background(), []byte("EICAR-DATA"), "eicar.txt")
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

	t.Run("empty data returns validation error", func(t *testing.T) {
		env := newTestEnv(t, &mockClamAVServer{})
		defer env.close()

		_, err := env.client.ScanFile(context.Background(), []byte{}, "empty.txt")
		if err == nil {
			t.Fatal("expected error for empty data")
		}
		if !clamav.IsValidationError(err) {
			t.Errorf("expected validation error, got: %T %v", err, err)
		}
	})

	t.Run("scan error from server", func(t *testing.T) {
		env := newTestEnv(t, &mockClamAVServer{
			scanFunc: func(data []byte, filename string) (*pb.ScanResponse, error) {
				return nil, status.Error(codes.Internal, "scan failed: clamd unavailable")
			},
		})
		defer env.close()

		_, err := env.client.ScanFile(context.Background(), []byte("data"), "test.txt")
		if err == nil {
			t.Fatal("expected error")
		}
		if !clamav.IsServiceError(err) {
			t.Errorf("expected service error, got: %T %v", err, err)
		}
	})
}

// --- ScanFilePath tests ---

func TestScanFilePath(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		env := newTestEnv(t, &mockClamAVServer{})
		defer env.close()

		// Create temp file
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "test.txt")
		if err := os.WriteFile(tmpFile, []byte("test content"), 0644); err != nil {
			t.Fatalf("write temp file: %v", err)
		}

		result, err := env.client.ScanFilePath(context.Background(), tmpFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsClean() {
			t.Errorf("expected clean, got status %q", result.Status)
		}
		if result.Filename != "test.txt" {
			t.Errorf("Filename = %q, want %q", result.Filename, "test.txt")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		env := newTestEnv(t, &mockClamAVServer{})
		defer env.close()

		_, err := env.client.ScanFilePath(context.Background(), "/nonexistent/file.txt")
		if err == nil {
			t.Fatal("expected error")
		}
		if !clamav.IsValidationError(err) {
			t.Errorf("expected validation error, got: %T %v", err, err)
		}
	})
}

// --- ScanStream tests ---

func TestScanStream(t *testing.T) {
	t.Run("clean data", func(t *testing.T) {
		env := newTestEnv(t, &mockClamAVServer{})
		defer env.close()

		data := []byte("clean stream data")
		result, err := env.client.ScanStream(context.Background(), data, "stream.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsClean() {
			t.Errorf("expected clean, got status %q", result.Status)
		}
		if result.Filename != "stream.txt" {
			t.Errorf("Filename = %q, want %q", result.Filename, "stream.txt")
		}
	})

	t.Run("chunked large data", func(t *testing.T) {
		var receivedData []byte
		env := newTestEnv(t, &mockClamAVServer{
			scanFunc: func(data []byte, filename string) (*pb.ScanResponse, error) {
				receivedData = data
				return &pb.ScanResponse{Status: "OK", Filename: filename}, nil
			},
		})
		defer env.close()

		// Use small chunk size for testing
		env.client.chunkSize = 10

		data := bytes.Repeat([]byte("A"), 35)
		result, err := env.client.ScanStream(context.Background(), data, "large.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsClean() {
			t.Errorf("expected clean result")
		}
		if !bytes.Equal(receivedData, data) {
			t.Errorf("received data length = %d, want %d", len(receivedData), len(data))
		}
	})

	t.Run("empty data", func(t *testing.T) {
		env := newTestEnv(t, &mockClamAVServer{})
		defer env.close()

		result, err := env.client.ScanStream(context.Background(), []byte{}, "empty.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Filename != "empty.txt" {
			t.Errorf("Filename = %q, want %q", result.Filename, "empty.txt")
		}
	})
}

// --- ScanStreamReader tests ---

func TestScanStreamReader(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		env := newTestEnv(t, &mockClamAVServer{})
		defer env.close()

		reader := strings.NewReader("stream reader data")
		result, err := env.client.ScanStreamReader(context.Background(), reader, "reader.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsClean() {
			t.Errorf("expected clean, got status %q", result.Status)
		}
	})

	t.Run("large reader chunked", func(t *testing.T) {
		var receivedLen int
		env := newTestEnv(t, &mockClamAVServer{
			scanFunc: func(data []byte, filename string) (*pb.ScanResponse, error) {
				receivedLen = len(data)
				return &pb.ScanResponse{Status: "OK", Filename: filename}, nil
			},
		})
		defer env.close()

		env.client.chunkSize = 16

		data := bytes.Repeat([]byte("B"), 100)
		result, err := env.client.ScanStreamReader(context.Background(), bytes.NewReader(data), "big.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsClean() {
			t.Errorf("expected clean result")
		}
		if receivedLen != 100 {
			t.Errorf("received len = %d, want 100", receivedLen)
		}
	})

	t.Run("empty reader", func(t *testing.T) {
		env := newTestEnv(t, &mockClamAVServer{})
		defer env.close()

		result, err := env.client.ScanStreamReader(context.Background(), strings.NewReader(""), "empty.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Filename != "empty.txt" {
			t.Errorf("Filename = %q, want %q", result.Filename, "empty.txt")
		}
	})
}

// --- ScanStreamFile tests ---

func TestScanStreamFile(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		env := newTestEnv(t, &mockClamAVServer{})
		defer env.close()

		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "streamfile.txt")
		if err := os.WriteFile(tmpFile, []byte("file content for streaming"), 0644); err != nil {
			t.Fatalf("write temp file: %v", err)
		}

		result, err := env.client.ScanStreamFile(context.Background(), tmpFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsClean() {
			t.Errorf("expected clean result")
		}
		if result.Filename != "streamfile.txt" {
			t.Errorf("Filename = %q, want %q", result.Filename, "streamfile.txt")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		env := newTestEnv(t, &mockClamAVServer{})
		defer env.close()

		_, err := env.client.ScanStreamFile(context.Background(), "/nonexistent/file.txt")
		if err == nil {
			t.Fatal("expected error")
		}
		if !clamav.IsValidationError(err) {
			t.Errorf("expected validation error, got: %v", err)
		}
	})
}

// --- ScanMultiple tests ---

func TestScanMultiple(t *testing.T) {
	t.Run("multiple clean files", func(t *testing.T) {
		env := newTestEnv(t, &mockClamAVServer{})
		defer env.close()

		files := []clamav.FileInput{
			{Data: []byte("file1 content"), Filename: "file1.txt"},
			{Data: []byte("file2 content"), Filename: "file2.txt"},
			{Data: []byte("file3 content"), Filename: "file3.txt"},
		}

		results, err := env.client.ScanMultiple(context.Background(), files)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var count int
		for result := range results {
			if !result.IsClean() {
				t.Errorf("expected clean, got status %q for %s", result.Status, result.Filename)
			}
			count++
		}
		if count != 3 {
			t.Errorf("got %d results, want 3", count)
		}
	})

	t.Run("mixed clean and infected", func(t *testing.T) {
		env := newTestEnv(t, &mockClamAVServer{
			scanFunc: func(data []byte, filename string) (*pb.ScanResponse, error) {
				if strings.Contains(string(data), "EICAR") {
					return &pb.ScanResponse{
						Status:   "FOUND",
						Message:  "Eicar-Test-Signature",
						Filename: filename,
					}, nil
				}
				return &pb.ScanResponse{Status: "OK", Filename: filename}, nil
			},
		})
		defer env.close()

		files := []clamav.FileInput{
			{Data: []byte("clean data"), Filename: "clean.txt"},
			{Data: []byte("EICAR-DATA"), Filename: "infected.txt"},
		}

		results, err := env.client.ScanMultiple(context.Background(), files)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var cleanCount, infectedCount int
		for result := range results {
			switch {
			case result.IsClean():
				cleanCount++
			case result.IsInfected():
				infectedCount++
			}
		}
		if cleanCount != 1 {
			t.Errorf("cleanCount = %d, want 1", cleanCount)
		}
		if infectedCount != 1 {
			t.Errorf("infectedCount = %d, want 1", infectedCount)
		}
	})

	t.Run("empty file list", func(t *testing.T) {
		env := newTestEnv(t, &mockClamAVServer{})
		defer env.close()

		results, err := env.client.ScanMultiple(context.Background(), []clamav.FileInput{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		count := 0
		for range results {
			count++
		}
		if count != 0 {
			t.Errorf("got %d results for empty input, want 0", count)
		}
	})
}

// --- ScanMultipleCallback tests ---

func TestScanMultipleCallback(t *testing.T) {
	t.Run("callback invoked per result", func(t *testing.T) {
		env := newTestEnv(t, &mockClamAVServer{})
		defer env.close()

		files := []clamav.FileInput{
			{Data: []byte("file1"), Filename: "f1.txt"},
			{Data: []byte("file2"), Filename: "f2.txt"},
		}

		var results []*clamav.ScanResult
		err := env.client.ScanMultipleCallback(context.Background(), files, func(r *clamav.ScanResult) {
			results = append(results, r)
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("got %d results, want 2", len(results))
		}
	})
}

// --- Context cancellation tests ---

func TestContextCancellation(t *testing.T) {
	t.Run("scan file with canceled context", func(t *testing.T) {
		env := newTestEnv(t, &mockClamAVServer{
			scanFunc: func(data []byte, filename string) (*pb.ScanResponse, error) {
				time.Sleep(2 * time.Second)
				return &pb.ScanResponse{Status: "OK"}, nil
			},
		})
		defer env.close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := env.client.ScanFile(ctx, []byte("data"), "test.txt")
		if err == nil {
			t.Fatal("expected error")
		}
		if !clamav.IsTimeoutError(err) {
			t.Errorf("expected timeout error, got: %T %v", err, err)
		}
	})

	t.Run("deadline exceeded", func(t *testing.T) {
		env := newTestEnv(t, &mockClamAVServer{
			scanFunc: func(data []byte, filename string) (*pb.ScanResponse, error) {
				time.Sleep(2 * time.Second)
				return &pb.ScanResponse{Status: "OK"}, nil
			},
		})
		defer env.close()

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, err := env.client.ScanFile(ctx, []byte("data"), "test.txt")
		if err == nil {
			t.Fatal("expected error")
		}
		if !clamav.IsTimeoutError(err) {
			t.Errorf("expected timeout error, got: %T %v", err, err)
		}
	})
}

// --- Error mapping tests ---

func TestMapGRPCError(t *testing.T) {
	tests := []struct {
		name   string
		code   codes.Code
		check  func(error) bool
		label  string
	}{
		{"InvalidArgument", codes.InvalidArgument, clamav.IsValidationError, "validation"},
		{"Internal", codes.Internal, clamav.IsServiceError, "service"},
		{"DeadlineExceeded", codes.DeadlineExceeded, clamav.IsTimeoutError, "timeout"},
		{"Canceled", codes.Canceled, clamav.IsTimeoutError, "timeout"},
		{"Unavailable", codes.Unavailable, clamav.IsConnectionError, "connection"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grpcErr := status.Error(tt.code, "test error")
			sdkErr := mapGRPCError(grpcErr)
			if !tt.check(sdkErr) {
				t.Errorf("expected %s error, got: %T %v", tt.label, sdkErr, sdkErr)
			}
		})
	}

	t.Run("nil error", func(t *testing.T) {
		if err := mapGRPCError(nil); err != nil {
			t.Errorf("expected nil, got: %v", err)
		}
	})

	t.Run("unknown code maps to service error", func(t *testing.T) {
		grpcErr := status.Error(codes.DataLoss, "data lost")
		sdkErr := mapGRPCError(grpcErr)
		if !clamav.IsServiceError(sdkErr) {
			t.Errorf("expected service error for unknown code, got: %T %v", sdkErr, sdkErr)
		}
	})
}

// --- Client option tests ---

func TestClientOptions(t *testing.T) {
	t.Run("custom chunk size", func(t *testing.T) {
		var chunkCount int
		env := newTestEnv(t, &mockClamAVServer{
			scanFunc: func(data []byte, filename string) (*pb.ScanResponse, error) {
				return &pb.ScanResponse{Status: "OK", Filename: filename}, nil
			},
		})
		defer env.close()

		// Override with small chunk size
		env.client.chunkSize = 5

		// Intercept via scan func to count chunks won't work here since
		// the mock receives assembled data. Instead just verify it works.
		data := bytes.Repeat([]byte("X"), 23)
		result, err := env.client.ScanStream(context.Background(), data, "chunked.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsClean() {
			t.Errorf("expected clean result")
		}
		_ = chunkCount
	})

	t.Run("WithTimeout option", func(t *testing.T) {
		c := &Client{}
		WithTimeout(42 * time.Second)(c)
		if c.timeout != 42*time.Second {
			t.Errorf("timeout = %v, want 42s", c.timeout)
		}
	})

	t.Run("WithChunkSize zero ignored", func(t *testing.T) {
		c := &Client{chunkSize: 100}
		WithChunkSize(0)(c)
		if c.chunkSize != 100 {
			t.Errorf("chunkSize = %d, want 100", c.chunkSize)
		}
	})

	t.Run("WithMaxMessageSize zero ignored", func(t *testing.T) {
		c := &Client{maxMessageSize: 100}
		WithMaxMessageSize(0)(c)
		if c.maxMessageSize != 100 {
			t.Errorf("maxMessageSize = %d, want 100", c.maxMessageSize)
		}
	})
}

// --- Close tests ---

func TestClose(t *testing.T) {
	t.Run("close with connection", func(t *testing.T) {
		env := newTestEnv(t, &mockClamAVServer{})
		defer env.close()

		err := env.client.Close()
		if err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	t.Run("close with nil connection", func(t *testing.T) {
		c := &Client{}
		err := c.Close()
		if err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
}

// --- Concurrent usage test ---

func TestConcurrentUsage(t *testing.T) {
	env := newTestEnv(t, &mockClamAVServer{})
	defer env.close()

	ctx := context.Background()
	errs := make(chan error, 20)

	for i := 0; i < 10; i++ {
		go func() {
			_, err := env.client.ScanFile(ctx, []byte("data"), "test.txt")
			errs <- err
		}()
		go func() {
			_, err := env.client.HealthCheck(ctx)
			errs <- err
		}()
	}

	for i := 0; i < 20; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent request failed: %v", err)
		}
	}
}
