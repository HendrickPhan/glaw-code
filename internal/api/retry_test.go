package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNetworkErrorRetry(t *testing.T) {
	tests := []struct {
		name              string
		statusCode        int
		responseBody      string
		expectRetry       bool
		retryCount        int
		expectedErrorType ErrorType
	}{
		{
			name:              "network error 400 should retry",
			statusCode:        400,
			responseBody:      `{"type":"error","error":{"message":"Network error, error id: 12345, please contact customer service","code":"1234"},"request_id":"12345"}`,
			expectRetry:       true,
			retryCount:        3, // initial + 2 retries
			expectedErrorType:  ErrRetriesExhausted,
		},
		{
			name:              "connection error 400 should retry",
			statusCode:        400,
			responseBody:      `{"error":"connection error"}`,
			expectRetry:       true,
			retryCount:        3,
			expectedErrorType:  ErrRetriesExhausted,
		},
		{
			name:              "timeout error 400 should retry",
			statusCode:        400,
			responseBody:      `{"error":"timeout occurred"}`,
			expectRetry:       true,
			retryCount:        3,
			expectedErrorType:  ErrRetriesExhausted,
		},
		{
			name:              "rate limit 429 should retry",
			statusCode:        429,
			responseBody:      `{"error":"rate limit exceeded"}`,
			expectRetry:       true,
			retryCount:        3,
			expectedErrorType:  ErrRetriesExhausted,
		},
		{
			name:              "server error 500 should retry",
			statusCode:        500,
			responseBody:      `{"error":"internal server error"}`,
			expectRetry:       true,
			retryCount:        3,
			expectedErrorType:  ErrRetriesExhausted,
		},
		{
			name:              "bad request 400 without network error should not retry",
			statusCode:        400,
			responseBody:      `{"error":"invalid request"}`,
			expectRetry:       false,
			retryCount:        1,
			expectedErrorType:  ErrHTTP,
		},
		{
			name:              "unauthorized 401 should not retry",
			statusCode:        401,
			responseBody:      `{"error":"unauthorized"}`,
			expectRetry:       false,
			retryCount:        1,
			expectedErrorType:  ErrHTTP,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attemptCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attemptCount++
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.responseBody))
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

			if tt.expectRetry {
				if attemptCount != tt.retryCount {
					t.Errorf("expected %d retry attempts, got %d", tt.retryCount, attemptCount)
				}
			} else {
				if attemptCount != 1 {
					t.Errorf("expected no retry (1 attempt only), got %d attempts", attemptCount)
				}
			}

			if err == nil {
				t.Fatal("expected error, got nil")
			}

			apiErr, ok := err.(*Error)
			if !ok {
				t.Fatalf("expected *api.Error, got %T", err)
			}

			if apiErr.Type != tt.expectedErrorType {
				t.Errorf("expected error type %v, got %v", tt.expectedErrorType, apiErr.Type)
			}

			if resp != nil {
				t.Error("expected nil response, got non-nil")
			}
		})
	}
}

func TestOpenAINetworkErrorRetry(t *testing.T) {
	tests := []struct {
		name              string
		statusCode        int
		responseBody      string
		expectRetry       bool
		retryCount        int
		expectedErrorType ErrorType
	}{
		{
			name:              "network error 400 should retry",
			statusCode:        400,
			responseBody:      `{"type":"error","error":{"message":"Network error, error id: 12345, please contact customer service","code":"1234"},"request_id":"12345"}`,
			expectRetry:       true,
			retryCount:        3, // OpenAI defaults to MaxRetries=2 (so 3 attempts total: initial + 2 retries)
			expectedErrorType:  ErrRetriesExhausted,
		},
		{
			name:              "bad request 400 without network error should not retry",
			statusCode:        400,
			responseBody:      `{"error":"invalid request"}`,
			expectRetry:       false,
			retryCount:        1,
			expectedErrorType:  ErrHTTP,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attemptCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attemptCount++
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			client := NewOpenAICompatClient(ClientConfig{
				APIKey:     "test-key",
				BaseURL:    server.URL,
				MaxRetries: 2,
			})

			req := Request{
				Model:     "gpt-4",
				Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("hello")}}},
				MaxTokens: 100,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			resp, err := client.SendMessage(ctx, req)

			if tt.expectRetry {
				if attemptCount != tt.retryCount {
					t.Errorf("expected %d retry attempts, got %d", tt.retryCount, attemptCount)
				}
			} else {
				if attemptCount != 1 {
					t.Errorf("expected no retry (1 attempt only), got %d attempts", attemptCount)
				}
			}

			if err == nil {
				t.Fatal("expected error, got nil")
			}

			apiErr, ok := err.(*Error)
			if !ok {
				t.Fatalf("expected *api.Error, got %T", err)
			}

			if apiErr.Type != tt.expectedErrorType {
				t.Errorf("expected error type %v, got %v", tt.expectedErrorType, apiErr.Type)
			}

			if resp != nil {
				t.Error("expected nil response, got non-nil")
			}
		})
	}
}

func TestIsNetworkErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		expected bool
	}{
		{"network error", "Network error, error id: 12345", true},
		{"connection error", "connection error occurred", true},
		{"timeout", "request timeout", true},
		{"temporary failure", "temporary failure", true},
		{"service unavailable", "service unavailable", true},
		{"gateway timeout", "gateway timeout", true},
		{"bad gateway", "bad gateway", true},
		{"invalid request", "invalid request", false},
		{"unauthorized", "unauthorized access", false},
		{"not found", "resource not found", false},
		{"empty string", "", false},
		{"case insensitive", "NETWORK ERROR", true},
		{"partial match", "there was a network error in the system", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNetworkErrorMessage(tt.message)
			if result != tt.expected {
				t.Errorf("isNetworkErrorMessage(%q) = %v, want %v", tt.message, result, tt.expected)
			}
		})
	}
}

func TestSuccessfulRetry(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 3 {
			// Fail first 2 attempts
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"type":"error","error":{"message":"Network error, error id: 12345","code":"1234"}}`))
			return
		}
		// Succeed on 3rd attempt
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg_123",
			"type":"message",
			"role":"assistant",
			"content":[{"type":"text","text":"Hello!"}],
			"stop_reason":"end_turn",
			"usage":{"input_tokens":10,"output_tokens":5}
		}`))
	}))
	defer server.Close()

	client := NewAnthropicClient(ClientConfig{
		APIKey:     "test-key",
		BaseURL:    server.URL,
		MaxRetries: 3, // Allow enough retries
	})

	req := Request{
		Model:     "claude-3-sonnet-20240229",
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("hello")}}},
		MaxTokens: 100,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.SendMessage(ctx, req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp == nil {
		t.Fatal("expected response, got nil")
	}

	if attemptCount != 3 {
		t.Errorf("expected 3 attempts (2 failures + 1 success), got %d", attemptCount)
	}

	if len(resp.Content) == 0 {
		t.Error("expected content in response")
	} else if resp.Content[0].Text != "Hello!" {
		t.Errorf("expected response text 'Hello!', got '%s'", resp.Content[0].Text)
	}
}

func TestRetryWithCancellation(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":"Network error"}`))
	}))
	defer server.Close()

	client := NewAnthropicClient(ClientConfig{
		APIKey:     "test-key",
		BaseURL:    server.URL,
		MaxRetries: 10,
	})

	req := Request{
		Model:     "claude-3-sonnet-20240229",
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("hello")}}},
		MaxTokens: 100,
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after first attempt
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	_, err := client.SendMessage(ctx, req)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err != context.Canceled {
		t.Errorf("expected context.Canceled error, got %v", err)
	}

	// Should have made at most 1-2 attempts before cancellation
	if attemptCount > 2 {
		t.Errorf("expected at most 2 attempts before cancellation, got %d", attemptCount)
	}
}
