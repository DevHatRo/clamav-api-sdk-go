package clamav

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{
			name: "without cause",
			err:  &Error{Code: CodeConnection, Message: "connection refused"},
			want: "connection refused",
		},
		{
			name: "with cause",
			err:  &Error{Code: CodeConnection, Message: "connection refused", Cause: errors.New("dial tcp")},
			want: "connection refused: dial tcp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestErrorUnwrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := &Error{Code: CodeTimeout, Message: "timed out", Cause: cause}

	if !errors.Is(err, cause) {
		t.Error("errors.Is should find the cause")
	}

	err2 := &Error{Code: CodeTimeout, Message: "timed out"}
	if err2.Unwrap() != nil {
		t.Error("Unwrap should return nil when no cause")
	}
}

func TestErrorAs(t *testing.T) {
	err := NewConnectionError("connection refused", nil)
	wrapped := fmt.Errorf("request failed: %w", err)

	var target *Error
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As should find *Error")
	}
	if target.Code != CodeConnection {
		t.Errorf("Code = %q, want %q", target.Code, CodeConnection)
	}
}

func TestNewConnectionError(t *testing.T) {
	cause := errors.New("dial tcp: connection refused")
	err := NewConnectionError("cannot reach server", cause)

	if err.Code != CodeConnection {
		t.Errorf("Code = %q, want %q", err.Code, CodeConnection)
	}
	if err.Message != "cannot reach server" {
		t.Errorf("Message = %q, want %q", err.Message, "cannot reach server")
	}
	if err.Cause != cause {
		t.Error("Cause should be the provided error")
	}
}

func TestNewTimeoutError(t *testing.T) {
	err := NewTimeoutError("scan timed out", nil)
	if err.Code != CodeTimeout {
		t.Errorf("Code = %q, want %q", err.Code, CodeTimeout)
	}
}

func TestNewValidationError(t *testing.T) {
	err := NewValidationError("file too large", nil)
	if err.Code != CodeValidation {
		t.Errorf("Code = %q, want %q", err.Code, CodeValidation)
	}
}

func TestNewServiceError(t *testing.T) {
	err := NewServiceError("clamd unavailable", 502, nil)
	if err.Code != CodeService {
		t.Errorf("Code = %q, want %q", err.Code, CodeService)
	}
	if err.StatusCode != 502 {
		t.Errorf("StatusCode = %d, want %d", err.StatusCode, 502)
	}
}

func TestIsConnectionError(t *testing.T) {
	err := NewConnectionError("conn failed", nil)
	if !IsConnectionError(err) {
		t.Error("IsConnectionError should return true")
	}
	if !IsConnectionError(fmt.Errorf("wrapped: %w", err)) {
		t.Error("IsConnectionError should work through wrapping")
	}
	if IsConnectionError(NewTimeoutError("timeout", nil)) {
		t.Error("IsConnectionError should return false for timeout errors")
	}
	if IsConnectionError(errors.New("random error")) {
		t.Error("IsConnectionError should return false for non-SDK errors")
	}
}

func TestIsTimeoutError(t *testing.T) {
	err := NewTimeoutError("timed out", nil)
	if !IsTimeoutError(err) {
		t.Error("IsTimeoutError should return true")
	}
	if !IsTimeoutError(fmt.Errorf("wrapped: %w", err)) {
		t.Error("IsTimeoutError should work through wrapping")
	}
	if IsTimeoutError(NewConnectionError("conn", nil)) {
		t.Error("IsTimeoutError should return false for connection errors")
	}
}

func TestIsValidationError(t *testing.T) {
	err := NewValidationError("bad input", nil)
	if !IsValidationError(err) {
		t.Error("IsValidationError should return true")
	}
	if !IsValidationError(fmt.Errorf("wrapped: %w", err)) {
		t.Error("IsValidationError should work through wrapping")
	}
	if IsValidationError(NewServiceError("svc", 500, nil)) {
		t.Error("IsValidationError should return false for service errors")
	}
}

func TestIsServiceError(t *testing.T) {
	err := NewServiceError("service down", 502, nil)
	if !IsServiceError(err) {
		t.Error("IsServiceError should return true")
	}
	if !IsServiceError(fmt.Errorf("wrapped: %w", err)) {
		t.Error("IsServiceError should work through wrapping")
	}
	if IsServiceError(NewValidationError("val", nil)) {
		t.Error("IsServiceError should return false for validation errors")
	}
}
