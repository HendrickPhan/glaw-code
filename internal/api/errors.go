package api

import (
	"fmt"
	"strings"
)

// ErrorType categorizes API errors.
type ErrorType int

const (
	ErrHTTP          ErrorType = iota
	ErrJSON
	ErrRateLimit
	ErrAuthentication
	ErrInvalidRequest
	ErrServerError
	ErrTimeout
	ErrSSE
	ErrRetriesExhausted
	ErrMissingCredentials
)

// Error represents an API-level error.
type Error struct {
	Type    ErrorType
	Message string
	Status  int
	Attempts int
	LastErr  error
}

func (e *Error) Error() string {
	switch e.Type {
	case ErrRetriesExhausted:
		return fmt.Sprintf("retries exhausted after %d attempts: %v", e.Attempts, e.LastErr)
	case ErrMissingCredentials:
		return fmt.Sprintf("missing credentials: %s", e.Message)
	default:
		if e.Status > 0 {
			return fmt.Sprintf("api error (status %d): %s", e.Status, e.Message)
		}
		return fmt.Sprintf("api error: %s", e.Message)
	}
}

func (e *Error) Unwrap() error {
	return e.LastErr
}

// NewHTTPError creates an HTTP error.
func NewHTTPError(status int, msg string) *Error {
	return &Error{Type: ErrHTTP, Status: status, Message: msg}
}

// NewRateLimitError creates a rate limit error.
func NewRateLimitError(msg string) *Error {
	return &Error{Type: ErrRateLimit, Message: msg}
}

// NewAuthenticationError creates an auth error.
func NewAuthenticationError(msg string) *Error {
	return &Error{Type: ErrAuthentication, Message: msg}
}

// NewRetriesExhaustedError creates a retries-exhausted error.
func NewRetriesExhaustedError(attempts int, lastErr error) *Error {
	return &Error{Type: ErrRetriesExhausted, Attempts: attempts, LastErr: lastErr}
}

// NewMissingCredentialsError creates a missing-credentials error.
func NewMissingCredentialsError(msg string) *Error {
	return &Error{Type: ErrMissingCredentials, Message: msg}
}

// IsRetryable returns true if the error can be retried.
func (e *Error) IsRetryable() bool {
	switch e.Type {
	case ErrRateLimit, ErrTimeout, ErrServerError:
		return true
	case ErrHTTP:
		// Retry on rate limiting, server errors, and network errors (400 with "Network error" message)
		if e.Status == 429 || e.Status == 503 || e.Status == 529 || e.Status >= 500 {
			return true
		}
		// Check for network error in status 400 responses
		if e.Status == 400 && isNetworkErrorMessage(e.Message) {
			return true
		}
		return false
	default:
		return false
	}
}

// isNetworkErrorMessage checks if the error message contains network-related error indicators
func isNetworkErrorMessage(msg string) bool {
	lower := strings.ToLower(msg)
	networkIndicators := []string{
		"network error",
		"connection error",
		"timeout",
		"temporary failure",
		"service unavailable",
		"gateway timeout",
		"bad gateway",
	}
	for _, indicator := range networkIndicators {
		if strings.Contains(lower, indicator) {
			return true
		}
	}
	return false
}
