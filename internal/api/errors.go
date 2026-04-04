package api

import "fmt"

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
	case ErrRateLimit:
		return true
	case ErrHTTP:
		return e.Status == 429 || e.Status == 503 || e.Status == 529
	default:
		return false
	}
}
