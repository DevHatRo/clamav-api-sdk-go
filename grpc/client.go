package grpc

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"time"

	clamav "github.com/DevHatRo/clamav-api-sdk-go"
	pb "github.com/DevHatRo/clamav-api-sdk-go/grpc/proto"
	grpclib "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// Client is the gRPC client for the ClamAV API.
// It is safe for concurrent use from multiple goroutines.
type Client struct {
	conn              *grpclib.ClientConn
	scanner           pb.ClamAVScannerClient
	timeout           time.Duration
	chunkSize         int
	maxMessageSize    int
	dialOpts          []grpclib.DialOption
	hasTransportCreds bool
}

// NewClient creates a gRPC client for the ClamAV API.
// target is the gRPC server address, e.g. "localhost:9000".
// By default, the connection uses insecure credentials. Use WithDialOptions
// to provide custom transport credentials.
func NewClient(target string, opts ...ClientOption) (*Client, error) {
	c := &Client{
		timeout:        defaultTimeout,
		chunkSize:      defaultChunkSize,
		maxMessageSize: defaultMaxMessageSize,
	}

	for _, opt := range opts {
		opt(c)
	}

	// Default to insecure only when caller did not set transport credentials (e.g. via WithTransportCredentials).
	if !c.hasTransportCreds {
		c.dialOpts = append(c.dialOpts, grpclib.WithTransportCredentials(insecure.NewCredentials()))
	}

	c.dialOpts = append(c.dialOpts,
		grpclib.WithDefaultCallOptions(
			grpclib.MaxCallRecvMsgSize(c.maxMessageSize),
			grpclib.MaxCallSendMsgSize(c.maxMessageSize),
		),
	)

	conn, err := grpclib.NewClient(target, c.dialOpts...)
	if err != nil {
		return nil, clamav.NewConnectionError("failed to create gRPC connection", err)
	}

	c.conn = conn
	c.scanner = pb.NewClamAVScannerClient(conn)

	return c, nil
}

// Close closes the gRPC connection.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// HealthCheck checks if the ClamAV service is healthy.
func (c *Client) HealthCheck(ctx context.Context) (*clamav.HealthCheckResult, error) {
	ctx, cancel := c.contextWithTimeout(ctx)
	defer cancel()

	resp, err := c.scanner.HealthCheck(ctx, &pb.HealthCheckRequest{})
	if err != nil {
		return nil, mapGRPCError(err)
	}

	return &clamav.HealthCheckResult{
		Healthy: resp.Status == "healthy",
		Message: resp.Message,
	}, nil
}

// ScanFile scans file data with a unary RPC call.
func (c *Client) ScanFile(ctx context.Context, data []byte, filename string) (*clamav.ScanResult, error) {
	if len(data) == 0 {
		return nil, mapGRPCError(status.Error(codes.InvalidArgument, "file data is required"))
	}
	ctx, cancel := c.contextWithTimeout(ctx)
	defer cancel()

	resp, err := c.scanner.ScanFile(ctx, &pb.ScanFileRequest{
		Data:     data,
		Filename: filename,
	})
	if err != nil {
		return nil, mapGRPCError(err)
	}

	return mapScanResponse(resp), nil
}

// ScanFilePath reads a file from disk and scans with a unary RPC.
func (c *Client) ScanFilePath(ctx context.Context, filePath string) (*clamav.ScanResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, clamav.NewValidationError("failed to read file: "+filePath, err)
	}
	return c.ScanFile(ctx, data, filepath.Base(filePath))
}

// ScanStream scans data via client streaming RPC.
// Chunks the data into pieces (configurable via WithChunkSize, default 64KB).
func (c *Client) ScanStream(ctx context.Context, data []byte, filename string) (*clamav.ScanResult, error) {
	ctx, cancel := c.contextWithTimeout(ctx)
	defer cancel()

	stream, err := c.scanner.ScanStream(ctx)
	if err != nil {
		return nil, mapGRPCError(err)
	}

	if err := c.sendChunks(stream, data, filename); err != nil {
		return nil, err
	}

	resp, err := stream.CloseAndRecv()
	if err != nil {
		return nil, mapGRPCError(err)
	}

	return mapScanResponse(resp), nil
}

// ScanStreamReader scans an io.Reader via client streaming RPC.
// Streams chunks without buffering the entire content in memory.
func (c *Client) ScanStreamReader(ctx context.Context, r io.Reader, filename string) (*clamav.ScanResult, error) {
	ctx, cancel := c.contextWithTimeout(ctx)
	defer cancel()

	stream, err := c.scanner.ScanStream(ctx)
	if err != nil {
		return nil, mapGRPCError(err)
	}

	buf := make([]byte, c.chunkSize)
	first := true

	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			req := &pb.ScanStreamRequest{
				Chunk: buf[:n],
			}
			if first {
				req.Filename = filename
				first = false
			}
			if readErr == io.EOF {
				req.IsLast = true
			}
			if err := stream.Send(req); err != nil {
				return nil, mapGRPCError(err)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, clamav.NewValidationError("failed to read data", readErr)
		}
	}

	// If no data was read at all, send a single empty last chunk
	if first {
		if err := stream.Send(&pb.ScanStreamRequest{
			Filename: filename,
			IsLast:   true,
		}); err != nil {
			return nil, mapGRPCError(err)
		}
	}

	resp, err := stream.CloseAndRecv()
	if err != nil {
		return nil, mapGRPCError(err)
	}

	return mapScanResponse(resp), nil
}

// ScanStreamFile reads a file from disk and scans via client streaming RPC.
func (c *Client) ScanStreamFile(ctx context.Context, filePath string) (*clamav.ScanResult, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, clamav.NewValidationError("failed to open file: "+filePath, err)
	}
	defer func() { _ = f.Close() }()

	return c.ScanStreamReader(ctx, f, filepath.Base(filePath))
}

// ScanMultiple scans multiple files using bidirectional streaming.
// Results are sent to the returned channel as they arrive.
// The channel is closed when all results have been received.
// Errors for individual files appear in ScanResult with Status "ERROR".
// If the consumer stops reading from the channel, goroutines exit on ctx.Done() so resources are not leaked.
func (c *Client) ScanMultiple(ctx context.Context, files []clamav.FileInput) (<-chan *clamav.ScanResult, error) {
	ctx, cancel := c.contextWithTimeout(ctx)

	stream, err := c.scanner.ScanMultiple(ctx)
	if err != nil {
		cancel()
		return nil, mapGRPCError(err)
	}

	bufSize := 2*len(files) + 1
	if bufSize < 1 {
		bufSize = 1
	}
	results := make(chan *clamav.ScanResult, bufSize)

	// sendResult sends to results or returns when ctx is done (avoids leaking if consumer stops reading).
	sendResult := func(r *clamav.ScanResult) bool {
		select {
		case results <- r:
			return true
		case <-ctx.Done():
			return false
		}
	}

	// Send all files
	go func() {
		defer func() {
			stream.CloseSend() //nolint:errcheck // best-effort on send side close
		}()

		for _, file := range files {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if err := c.sendChunks(stream, file.Data, file.Filename); err != nil {
				if !sendResult(&clamav.ScanResult{
					Status:   "ERROR",
					Message:  err.Error(),
					Filename: file.Filename,
				}) {
					return
				}
			}
		}
	}()

	// Receive results
	go func() {
		defer close(results)
		defer cancel()

		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				st, ok := status.FromError(err)
				if ok && st.Code() == codes.Canceled {
					return
				}
				sendResult(&clamav.ScanResult{
					Status:  "ERROR",
					Message: err.Error(),
				})
				return
			}
			if !sendResult(mapScanResponse(resp)) {
				return
			}
		}
	}()

	return results, nil
}

// ScanMultipleCallback is like ScanMultiple but invokes a callback for each result.
// Blocks until all results are received or ctx is canceled.
func (c *Client) ScanMultipleCallback(ctx context.Context, files []clamav.FileInput, fn func(*clamav.ScanResult)) error {
	results, err := c.ScanMultiple(ctx, files)
	if err != nil {
		return err
	}

	for result := range results {
		fn(result)
	}

	return nil
}

// sendChunks sends file data as chunks over a streaming RPC.
type chunkSender interface {
	Send(*pb.ScanStreamRequest) error
}

func (c *Client) sendChunks(stream chunkSender, data []byte, filename string) error {
	if len(data) == 0 {
		return stream.Send(&pb.ScanStreamRequest{
			Filename: filename,
			IsLast:   true,
		})
	}

	for i := 0; i < len(data); i += c.chunkSize {
		end := i + c.chunkSize
		if end > len(data) {
			end = len(data)
		}

		req := &pb.ScanStreamRequest{
			Chunk:  data[i:end],
			IsLast: end == len(data),
		}
		if i == 0 {
			req.Filename = filename
		}

		if err := stream.Send(req); err != nil {
			return mapGRPCError(err)
		}
	}

	return nil
}

// contextWithTimeout applies the default timeout if the context has no deadline.
func (c *Client) contextWithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, c.timeout)
}

// mapScanResponse converts a proto ScanResponse to a clamav.ScanResult.
func mapScanResponse(resp *pb.ScanResponse) *clamav.ScanResult {
	return &clamav.ScanResult{
		Status:   resp.Status,
		Message:  resp.Message,
		ScanTime: resp.ScanTime,
		Filename: resp.Filename,
	}
}

// mapGRPCError converts a gRPC error to an SDK error type.
func mapGRPCError(err error) error {
	if err == nil {
		return nil
	}

	st, ok := status.FromError(err)
	if !ok {
		return clamav.NewConnectionError("gRPC error", err)
	}

	switch st.Code() {
	case codes.InvalidArgument:
		return clamav.NewValidationError(st.Message(), err)
	case codes.Internal:
		return clamav.NewServiceError(st.Message(), 500, err)
	case codes.DeadlineExceeded:
		return clamav.NewTimeoutError(st.Message(), err)
	case codes.Canceled:
		return clamav.NewTimeoutError(st.Message(), err)
	case codes.Unavailable:
		return clamav.NewConnectionError(st.Message(), err)
	default:
		return clamav.NewServiceError(st.Message(), grpcCodeToHTTP(st.Code()), err)
	}
}

// grpcCodeToHTTP maps gRPC status codes to HTTP-equivalent status codes
// so that StatusCode is consistent between the REST and gRPC clients.
func grpcCodeToHTTP(c codes.Code) int {
	switch c {
	case codes.OK:
		return 200
	case codes.InvalidArgument:
		return 400
	case codes.Unauthenticated:
		return 401
	case codes.PermissionDenied:
		return 403
	case codes.NotFound:
		return 404
	case codes.AlreadyExists:
		return 409
	case codes.ResourceExhausted:
		return 429
	case codes.Canceled:
		return 499
	case codes.Internal, codes.DataLoss, codes.Unknown:
		return 500
	case codes.Unimplemented:
		return 501
	case codes.Unavailable:
		return 503
	case codes.DeadlineExceeded:
		return 504
	default:
		return 500
	}
}
