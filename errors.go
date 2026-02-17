package clamav

import (
	"errors"
	"fmt"
)

// Error codes for machine-readable error classification.
const (
	CodeConnection = "connection_error"
	CodeTimeout    = "timeout"
	CodeValidation = "validation_error"
	CodeService    = "service_error"
)

// Error is the base error type for all SDK errors.
type Error struct {
	// Code is a machine-readable error code.
	Code string
	// Message is a human-readable error description.
	Message string
	// StatusCode is the HTTP status code or gRPC status code.
	StatusCode int
	// Cause is the underlying error, if any.
	Cause error
}

// Error returns the human-readable error message.
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

// Unwrap returns the underlying cause for use with errors.Is and errors.As.
func (e *Error) Unwrap() error {
	return e.Cause
}

// NewConnectionError creates an error indicating a connection failure.
func NewConnectionError(msg string, cause error) *Error {
	return &Error{
		Code:    CodeConnection,
		Message: msg,
		Cause:   cause,
	}
}

// NewTimeoutError creates an error indicating a timeout.
func NewTimeoutError(msg string, cause error) *Error {
	return &Error{
		Code:    CodeTimeout,
		Message: msg,
		Cause:   cause,
	}
}

// NewValidationError creates an error indicating invalid input.
func NewValidationError(msg string, cause error) *Error {
	return &Error{
		Code:    CodeValidation,
		Message: msg,
		Cause:   cause,
	}
}

// NewServiceError creates an error indicating the ClamAV service is unavailable or errored.
func NewServiceError(msg string, statusCode int, cause error) *Error {
	return &Error{
		Code:       CodeService,
		Message:    msg,
		StatusCode: statusCode,
		Cause:      cause,
	}
}

// IsConnectionError reports whether err is or wraps a connection error.
func IsConnectionError(err error) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Code == CodeConnection
	}
	return false
}

// IsTimeoutError reports whether err is or wraps a timeout error.
func IsTimeoutError(err error) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Code == CodeTimeout
	}
	return false
}

// IsValidationError reports whether err is or wraps a validation error.
func IsValidationError(err error) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Code == CodeValidation
	}
	return false
}

// IsServiceError reports whether err is or wraps a service error.
func IsServiceError(err error) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Code == CodeService
	}
	return false
}
