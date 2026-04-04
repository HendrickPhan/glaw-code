package api

import (
	"errors"
	"testing"
)

func TestErrorError(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{
			"http error",
			NewHTTPError(400, "bad request"),
			"api error (status 400): bad request",
		},
		{
			"rate limit",
			NewRateLimitError("too many requests"),
			"api error: too many requests",
		},
		{
			"auth error",
			NewAuthenticationError("invalid key"),
			"api error: invalid key",
		},
		{
			"retries exhausted",
			NewRetriesExhaustedError(3, errors.New("timeout")),
			"retries exhausted after 3 attempts: timeout",
		},
		{
			"missing credentials",
			NewMissingCredentialsError("ANTHROPIC_API_KEY not set"),
			"missing credentials: ANTHROPIC_API_KEY not set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name      string
		err       *Error
		retryable bool
	}{
		{"rate limit", NewRateLimitError("rate limited"), true},
		{"429 http", NewHTTPError(429, "rate limit"), true},
		{"503 http", NewHTTPError(503, "overloaded"), true},
		{"529 http", NewHTTPError(529, "overloaded"), true},
		{"400 http", NewHTTPError(400, "bad request"), false},
		{"401 http", NewHTTPError(401, "unauthorized"), false},
		{"auth error", NewAuthenticationError("bad key"), false},
		{"missing creds", NewMissingCredentialsError("no key"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.IsRetryable() != tt.retryable {
				t.Errorf("IsRetryable() = %v, want %v", tt.err.IsRetryable(), tt.retryable)
			}
		})
	}
}

func TestErrorUnwrap(t *testing.T) {
	inner := errors.New("inner error")
	err := NewRetriesExhaustedError(2, inner)
	if unwrapped := err.Unwrap(); unwrapped != inner {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, inner)
	}
}
