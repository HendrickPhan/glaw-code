package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestProviderNetworkError tests the exact error scenario mentioned in the issue:
// "Error: api error (status 400): {"type":"error","error":{"message":"Network error, error id: 202604091754268b8c2879a8c24dfb, please contact customer service","code":"1234"},"request_id":"202604091754268b8c2879a8c24dfb"}"
func TestProviderNetworkError(t *testing.T) {
	// Simulate the exact error response from the provider
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		// Always return the network error
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{
			"type":"error",
			"error":{
				"message":"Network error, error id: 202604091754268b8c2879a8c24dfb, please contact customer service",
				"code":"1234"
			},
			"request_id":"202604091754268b8c2879a8c24dfb"
		}`))
	}))
	defer server.Close()

	client := NewAnthropicClient(ClientConfig{
		APIKey:     "test-key",
		BaseURL:    server.URL,
		MaxRetries: 2,
	})

	req := Request{
		Model:     "claude-3-sonnet-20240229",
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("hello")}}},
		MaxTokens: 100,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.SendMessage(ctx, req)

	// Verify retry behavior
	if attemptCount != 3 {
		t.Errorf("Expected 3 attempts (1 initial + 2 retries) for network error, got %d attempts", attemptCount)
	}

	// Verify error returned after retries exhausted
	if err == nil {
		t.Fatal("Expected error after retries exhausted, got nil")
	}

	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("Expected *api.Error, got %T", err)
	}

	// Verify it's a retries exhausted error
	if apiErr.Type != ErrRetriesExhausted {
		t.Errorf("Expected error type ErrRetriesExhausted, got %v", apiErr.Type)
	}

	// Verify the error contains information about retries
	if apiErr.Attempts != 3 {
		t.Errorf("Expected 3 attempts recorded in error, got %d", apiErr.Attempts)
	}

	// Verify response is nil after failed retries
	if resp != nil {
		t.Error("Expected nil response after failed retries, got non-nil")
	}

	t.Logf("✓ Successfully retried network error %d times before giving up", apiErr.Attempts)
}

// TestProviderNetworkErrorRecovery tests that the agent can recover when
// the provider starts responding successfully after network errors
func TestProviderNetworkErrorRecovery(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount <= 2 {
			// Fail with network error first 2 times
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{
				"type":"error",
				"error":{
					"message":"Network error, error id: 202604091754268b8c2879a8c24dfb, please contact customer service",
					"code":"1234"
				},
				"request_id":"202604091754268b8c2879a8c24dfb"
			}`))
			return
		}

		// Succeed on 3rd attempt
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id":"msg_123",
			"type":"message",
			"role":"assistant",
			"content":[{"type":"text","text":"Hello! I recovered from the network error."}],
			"stop_reason":"end_turn",
			"usage":{"input_tokens":10,"output_tokens":5}
		}`))
	}))
	defer server.Close()

	client := NewAnthropicClient(ClientConfig{
		APIKey:     "test-key",
		BaseURL:    server.URL,
		MaxRetries: 3,
	})

	req := Request{
		Model:     "claude-3-sonnet-20240229",
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("hello")}}},
		MaxTokens: 100,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.SendMessage(ctx, req)

	// Verify successful recovery
	if err != nil {
		t.Fatalf("Expected successful request after retries, got error: %v", err)
	}

	if resp == nil {
		t.Fatal("Expected response after successful retry, got nil")
	}

	// Verify it took exactly 3 attempts (2 failures + 1 success)
	if attemptCount != 3 {
		t.Errorf("Expected 3 attempts (2 failures + 1 success), got %d", attemptCount)
	}

	// Verify response content
	if len(resp.Content) == 0 {
		t.Error("Expected content in response, got empty")
	} else if resp.Content[0].Text != "Hello! I recovered from the network error." {
		t.Errorf("Expected recovery message, got: %s", resp.Content[0].Text)
	}

	t.Logf("✓ Successfully recovered from network error after %d attempts", attemptCount)
}

// TestDifferentProviderErrors tests that various provider error types
// are handled correctly with appropriate retry logic
func TestDifferentProviderErrors(t *testing.T) {
	tests := []struct {
		name              string
		statusCode        int
		responseBody      string
		expectRetry       bool
		expectedAttempts  int
	}{
		{
			name:         "Provider network error should retry",
			statusCode:   400,
			responseBody: `{"type":"error","error":{"message":"Network error, error id: abc123","code":"1234"}}`,
			expectRetry:  true,
			expectedAttempts: 3,
		},
		{
			name:         "Provider timeout should retry",
			statusCode:   408,
			responseBody: `{"error":"request timeout"}`,
			expectRetry:  true,
			expectedAttempts: 3,
		},
		{
			name:         "Provider rate limit should retry",
			statusCode:   429,
			responseBody: `{"error":"rate limit exceeded"}`,
			expectRetry:  true,
			expectedAttempts: 3,
		},
		{
			name:         "Provider server error should retry",
			statusCode:   503,
			responseBody: `{"error":"service unavailable"}`,
			expectRetry:  true,
			expectedAttempts: 3,
		},
		{
			name:         "Provider bad request should NOT retry",
			statusCode:   400,
			responseBody: `{"error":"bad request"}`,
			expectRetry:  false,
			expectedAttempts: 1,
		},
		{
			name:         "Provider unauthorized should NOT retry",
			statusCode:   401,
			responseBody: `{"error":"unauthorized"}`,
			expectRetry:  false,
			expectedAttempts: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attemptCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attemptCount++
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			client := NewAnthropicClient(ClientConfig{
				APIKey:     "test-key",
				BaseURL:    server.URL,
				MaxRetries: 2,
			})

			req := Request{
				Model:     "claude-3-sonnet-20240229",
				Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("test")}}},
				MaxTokens: 100,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			_, err := client.SendMessage(ctx, req)

			if attemptCount != tt.expectedAttempts {
				t.Errorf("Expected %d attempts, got %d", tt.expectedAttempts, attemptCount)
			}

			if err == nil {
				t.Fatal("Expected error for failing provider, got nil")
			}

			apiErr, ok := err.(*Error)
			if !ok {
				t.Fatalf("Expected *api.Error, got %T", err)
			}

			if tt.expectRetry {
				if apiErr.Type != ErrRetriesExhausted {
					t.Errorf("Expected ErrRetriesExhausted after retries, got %v", apiErr.Type)
				}
			} else {
				if apiErr.Type != ErrHTTP {
					t.Errorf("Expected ErrHTTP for non-retryable error, got %v", apiErr.Type)
				}
			}
		})
	}
}
