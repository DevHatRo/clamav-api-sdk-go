// Package testutil provides test helpers for the clamav-api-sdk-go SDK.
package testutil

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
)

// MockResponse defines a canned response for the mock server.
type MockResponse struct {
	StatusCode int
	Body       interface{}
}

// NewMockServer creates an httptest.Server that handles ClamAV API endpoints.
// The handlers map allows overriding behavior per endpoint path.
func NewMockServer(handlers map[string]http.HandlerFunc) *httptest.Server {
	mux := http.NewServeMux()
	for path, handler := range handlers {
		mux.HandleFunc(path, handler)
	}
	return httptest.NewServer(mux)
}

// JSONHandler returns an http.HandlerFunc that responds with the given status code and JSON body.
func JSONHandler(statusCode int, body interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(body) //nolint:errcheck
	}
}

// ScanHandler returns an http.HandlerFunc that reads the uploaded file and responds with a scan result.
// The checkFunc is called with the uploaded file data and can return a custom response.
func ScanHandler(checkFunc func(data []byte, filename string) (int, interface{})) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var data []byte
		var filename string

		contentType := r.Header.Get("Content-Type")
		if contentType == "application/octet-stream" {
			// Stream scan
			var err error
			data, err = io.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"message": "failed to read body"}) //nolint:errcheck
				return
			}
			filename = "stream"
		} else {
			// Multipart scan
			file, header, err := r.FormFile("file")
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"message": "Provide a single file"}) //nolint:errcheck
				return
			}
			defer file.Close()

			data, err = io.ReadAll(file)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			filename = header.Filename
		}

		statusCode, body := checkFunc(data, filename)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(body) //nolint:errcheck
	}
}

// CleanScanResponse returns a standard clean scan response.
func CleanScanResponse() map[string]interface{} {
	return map[string]interface{}{
		"status":  "OK",
		"message": "",
		"time":    0.001234,
	}
}

// InfectedScanResponse returns a standard infected scan response.
func InfectedScanResponse() map[string]interface{} {
	return map[string]interface{}{
		"status":  "FOUND",
		"message": "Eicar-Test-Signature",
		"time":    0.002342,
	}
}
