// Package grpc provides a gRPC client for the ClamAV API antivirus scanning service.
//
// This sub-package depends on google.golang.org/grpc and google.golang.org/protobuf.
// If you only need REST, import the root package github.com/DevHatRo/clamav-api-sdk-go instead.
//
// # Quick Start
//
//	client, err := grpc.NewClient("localhost:9000")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
//
//	result, err := client.ScanFile(ctx, data, "test.txt")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Status: %s\n", result.Status)
package grpc
