// Package clamav provides a Go SDK for the ClamAV API antivirus scanning service.
//
// This package contains the REST client and shared types. It has zero external
// runtime dependencies (stdlib only).
//
// For gRPC support, import the sub-package github.com/DevHatRo/clamav-api-sdk-go/grpc.
//
// # Quick Start
//
//	client, err := clamav.NewClient("http://localhost:6000")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
//
//	result, err := client.ScanFilePath(ctx, "/path/to/file.pdf")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Status: %s, Infected: %v\n", result.Status, result.IsInfected())
package clamav
