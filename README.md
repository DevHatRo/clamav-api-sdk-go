# clamav-api-sdk-go

A production-ready Go SDK for the [ClamAV API](https://github.com/DevHatRo/ClamAV-API) antivirus scanning service. Supports both REST and gRPC transports.

**Repository:** [github.com/DevHatRo/clamav-api-sdk-go](https://github.com/DevHatRo/clamav-api-sdk-go)

```bash
# Clone
git clone git@github.com:DevHatRo/clamav-api-sdk-go.git

# Or add as remote to an existing repo
git remote add origin git@github.com:DevHatRo/clamav-api-sdk-go.git
```

## Features

- **REST client** with zero external runtime dependencies (stdlib only)
- **gRPC client** in a separate sub-module (no dependency bloat for REST-only users)
- File scanning via multipart upload, binary streaming, and gRPC streaming
- Bidirectional streaming for scanning multiple files in parallel (gRPC)
- Full `context.Context` support for cancellation and deadlines
- Typed errors with `IsConnectionError`, `IsTimeoutError`, `IsValidationError`, `IsServiceError` helpers
- Concurrent-safe clients
- Comprehensive test coverage with unit and integration tests

## Installation

### REST client only (zero external dependencies)

```bash
go get github.com/DevHatRo/clamav-api-sdk-go
```

### gRPC client

```bash
go get github.com/DevHatRo/clamav-api-sdk-go/grpc
```

## Quick Start

### REST Client

```go
package main

import (
    "context"
    "fmt"
    "log"

    clamav "github.com/DevHatRo/clamav-api-sdk-go"
)

func main() {
    client, err := clamav.NewClient("http://localhost:6000")
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    ctx := context.Background()

    // Health check
    health, err := client.HealthCheck(ctx)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Healthy: %v\n", health.Healthy)

    // Scan a file from disk
    result, err := client.ScanFilePath(ctx, "/path/to/file.pdf")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Status: %s, Infected: %v\n", result.Status, result.IsInfected())

    // Scan a byte slice
    result, err = client.ScanFile(ctx, []byte("file contents"), "test.txt")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Status: %s, Time: %.3fs\n", result.Status, result.ScanTime)

    // Stream scan a large file from disk
    result, err = client.StreamScanFile(ctx, "/path/to/large.iso")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Stream scan: %s\n", result.Status)
}
```

### REST Client - Scan from io.Reader

```go
resp, err := http.Get("https://example.com/file.bin")
if err != nil {
    log.Fatal(err)
}
defer resp.Body.Close()

result, err := client.ScanReader(ctx, resp.Body, "file.bin")
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Status: %s\n", result.Status)
```

### REST Client - Stream Scan with io.Reader

```go
f, _ := os.Open("/path/to/large.iso")
defer f.Close()

stat, _ := f.Stat()
result, err := client.StreamScan(ctx, f, stat.Size())
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Stream scan: %s\n", result.Status)
```

### gRPC Client

```go
package main

import (
    "context"
    "fmt"
    "log"

    clamav "github.com/DevHatRo/clamav-api-sdk-go"
    clamavgrpc "github.com/DevHatRo/clamav-api-sdk-go/grpc"
)

func main() {
    client, err := clamavgrpc.NewClient("localhost:9000")
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    ctx := context.Background()

    // Health check
    health, err := client.HealthCheck(ctx)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Healthy: %v\n", health.Healthy)

    // Scan file (unary RPC)
    result, err := client.ScanFile(ctx, []byte("file contents"), "test.txt")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Status: %s\n", result.Status)

    // Stream scan from file (client streaming RPC)
    result, err = client.ScanStreamFile(ctx, "/path/to/large.bin")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Stream: %s\n", result.Status)

    // Scan multiple files (bidirectional streaming)
    files := []clamav.FileInput{
        {Data: []byte("file1 contents"), Filename: "file1.txt"},
        {Data: []byte("file2 contents"), Filename: "file2.txt"},
    }

    results, err := client.ScanMultiple(ctx, files)
    if err != nil {
        log.Fatal(err)
    }
    for result := range results {
        fmt.Printf("%s: %s\n", result.Filename, result.Status)
    }
}
```

### gRPC Client - Scan Multiple with Callback

```go
err := client.ScanMultipleCallback(ctx, files, func(result *clamav.ScanResult) {
    if result.IsInfected() {
        fmt.Printf("INFECTED: %s - %s\n", result.Filename, result.Message)
    }
})
if err != nil {
    log.Fatal(err)
}
```

### Error Handling

```go
result, err := client.ScanFile(ctx, data, "test.txt")
if err != nil {
    switch {
    case clamav.IsTimeoutError(err):
        fmt.Println("Scan timed out, try again later")
    case clamav.IsServiceError(err):
        fmt.Println("ClamAV service is down")
    case clamav.IsConnectionError(err):
        fmt.Println("Cannot reach ClamAV API server")
    case clamav.IsValidationError(err):
        fmt.Println("Invalid input:", err)
    default:
        fmt.Println("Unexpected error:", err)
    }
    return
}
```

### Context Timeout

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

result, err := client.ScanFile(ctx, largeData, "big-file.bin")
if err != nil {
    if clamav.IsTimeoutError(err) {
        fmt.Println("Scan timed out")
    }
}
```

### Custom HTTP Client

```go
httpClient := &http.Client{
    Timeout: 60 * time.Second,
    Transport: &http.Transport{
        TLSClientConfig: &tls.Config{
            RootCAs: certPool,
        },
        MaxIdleConns:    10,
        IdleConnTimeout: 90 * time.Second,
    },
}

client, err := clamav.NewClient("https://clamav.example.com",
    clamav.WithHTTPClient(httpClient),
)
```

### Custom gRPC Options

```go
import "google.golang.org/grpc/credentials"

creds, err := credentials.NewClientTLSFromFile("ca.pem", "")
if err != nil {
    log.Fatal(err)
}

client, err := clamavgrpc.NewClient("clamav.example.com:9000",
    clamavgrpc.WithDialOptions(grpc.WithTransportCredentials(creds)),
    clamavgrpc.WithTimeout(60*time.Second),
    clamavgrpc.WithChunkSize(128*1024), // 128KB chunks
)
```

## API Reference

### REST Client Methods

| Method | Description |
|--------|-------------|
| `NewClient(baseURL, opts...)` | Create a new REST client |
| `HealthCheck(ctx)` | Check ClamAV service health |
| `Version(ctx)` | Get server version info |
| `ScanFile(ctx, data, filename)` | Scan bytes via multipart upload |
| `ScanFilePath(ctx, filePath)` | Scan a file from disk via multipart |
| `ScanReader(ctx, reader, filename)` | Scan an io.Reader via multipart |
| `StreamScan(ctx, reader, size)` | Scan via binary stream upload |
| `StreamScanFile(ctx, filePath)` | Stream scan a file from disk |
| `Close()` | Release client resources |

### gRPC Client Methods

| Method | Description |
|--------|-------------|
| `NewClient(target, opts...)` | Create a new gRPC client |
| `HealthCheck(ctx)` | Check ClamAV service health |
| `ScanFile(ctx, data, filename)` | Scan with unary RPC |
| `ScanFilePath(ctx, filePath)` | Read file and scan with unary RPC |
| `ScanStream(ctx, data, filename)` | Scan bytes with client streaming |
| `ScanStreamReader(ctx, reader, filename)` | Stream an io.Reader |
| `ScanStreamFile(ctx, filePath)` | Stream a file from disk |
| `ScanMultiple(ctx, files)` | Scan multiple files (bidi streaming) |
| `ScanMultipleCallback(ctx, files, fn)` | Scan multiple with callback |
| `Close()` | Close the gRPC connection |

## Development

### Prerequisites

- Go 1.22+
- Docker (for integration tests)
- protoc + protoc-gen-go + protoc-gen-go-grpc (for proto regeneration only)

### Running Tests

```bash
# Unit tests
make test

# Integration tests (requires running ClamAV API)
docker compose up -d
make test-integration

# Lint
make lint
```

### Running ClamAV API Locally

```bash
docker compose up
```

This starts the ClamAV API with:
- REST API on port 6000
- gRPC API on port 9000

Note: ClamAV needs ~60-120 seconds to download virus definitions on first start.

### Regenerating Proto

```bash
make proto
```

### Project Structure

```
clamav-api-sdk-go/
├── client.go                # REST client implementation
├── client_test.go           # REST client unit tests
├── errors.go                # Error types and helpers
├── errors_test.go           # Error tests
├── types.go                 # Shared types (ScanResult, etc.)
├── options.go               # REST client options
├── doc.go                   # Package documentation
├── integration_test.go      # REST integration tests
├── go.mod                   # Root module (stdlib only)
├── grpc/
│   ├── client.go            # gRPC client implementation
│   ├── client_test.go       # gRPC client unit tests
│   ├── options.go           # gRPC client options
│   ├── doc.go               # Sub-package docs
│   ├── integration_test.go  # gRPC integration tests
│   ├── go.mod               # Sub-module
│   └── proto/
│       ├── clamav.proto     # Proto definition
│       ├── clamav.pb.go     # Generated protobuf code
│       └── clamav_grpc.pb.go
├── internal/testutil/       # Test helpers
├── testdata/                # Test files (clean + EICAR)
├── docker-compose.yml       # Local ClamAV API
├── Makefile
├── .golangci.yml
└── .github/workflows/ci.yml # CI pipeline
```

## Versioning and releases

Releases are managed by [Release Please](https://github.com/googleapis/release-please). Merge the automated release PR to create a new release.

- Root module: `v1.x.x`
- gRPC sub-module: `grpc/v1.x.x`

## License

Apache License 2.0
