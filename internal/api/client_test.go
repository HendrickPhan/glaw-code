package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAnthropicClientSendMessage(t *testing.T) {
	responseBody := Response{
		ID:         "msg_123",
		StopReason: StopEndTurn,
		Usage:      Usage{InputTokens: 10, OutputTokens: 20},
		Content:    []ContentBlock{NewTextBlock("Hello!")},
	}
	body, _ := json.Marshal(responseBody)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("Path = %q, want /v1/messages", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("x-api-key = %q, want %q", r.Header.Get("x-api-key"), "test-key")
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("anthropic-version = %q", r.Header.Get("anthropic-version"))
		}

		w.Header().Set("x-request-id", "req_456")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer server.Close()

	client := NewAnthropicClient(ClientConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	req := Request{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 1024,
		Messages: []Message{
			{Role: RoleUser, Content: []ContentBlock{NewTextBlock("hi")}},
		},
	}

	resp, err := client.SendMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("SendMessage error: %v", err)
	}
	if resp.ID != "msg_123" {
		t.Errorf("ID = %q, want %q", resp.ID, "msg_123")
	}
	if resp.RequestID != "req_456" {
		t.Errorf("RequestID = %q, want %q", resp.RequestID, "req_456")
	}
	if resp.StopReason != StopEndTurn {
		t.Errorf("StopReason = %q, want %q", resp.StopReason, StopEndTurn)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("Content length = %d, want 1", len(resp.Content))
	}
	if resp.Content[0].Text != "Hello!" {
		t.Errorf("Content[0].Text = %q, want %q", resp.Content[0].Text, "Hello!")
	}
}

func TestAnthropicClientRetryOn429(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate limit"}`))
			return
		}
		resp := Response{
			ID:         "msg_ok",
			StopReason: StopEndTurn,
			Usage:      Usage{InputTokens: 5, OutputTokens: 5},
			Content:    []ContentBlock{NewTextBlock("retried!")},
		}
		body, _ := json.Marshal(resp)
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer server.Close()

	client := NewAnthropicClient(ClientConfig{
		APIKey:     "test-key",
		BaseURL:    server.URL,
		MaxRetries: 2,
	})

	req := Request{
		Model:     "test-model",
		MaxTokens: 100,
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("hi")}}},
	}

	resp, err := client.SendMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("SendMessage error: %v", err)
	}
	if resp.ID != "msg_ok" {
		t.Errorf("ID = %q, want %q", resp.ID, "msg_ok")
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestAnthropicClientRetriesExhausted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error":"overloaded"}`))
	}))
	defer server.Close()

	client := NewAnthropicClient(ClientConfig{
		APIKey:     "test-key",
		BaseURL:    server.URL,
		MaxRetries: 1,
	})

	req := Request{
		Model:     "test",
		MaxTokens: 100,
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("hi")}}},
	}

	_, err := client.SendMessage(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if apiErr.Type != ErrRetriesExhausted {
		t.Errorf("Error type = %v, want ErrRetriesExhausted", apiErr.Type)
	}
	if apiErr.Attempts != 2 {
		t.Errorf("Attempts = %d, want 2", apiErr.Attempts)
	}
}

func TestAnthropicClientNonRetryableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid key"}`))
	}))
	defer server.Close()

	client := NewAnthropicClient(ClientConfig{
		APIKey:     "test-key",
		BaseURL:    server.URL,
		MaxRetries: 3,
	})

	req := Request{
		Model:     "test",
		MaxTokens: 100,
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("hi")}}},
	}

	_, err := client.SendMessage(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if apiErr.Status != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", apiErr.Status, http.StatusUnauthorized)
	}
}

func TestAnthropicClientStreamMessage(t *testing.T) {
	sseData := `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":10}}}

event: content_block_start
data: {"type":"content_block_start","index":0}

event: content_block_delta
data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello"}}

event: content_block_delta
data: {"type":"content_block_delta","delta":{"type":"text_delta","text":" world"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}

event: message_stop
data: {"type":"message_stop"}
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	client := NewAnthropicClient(ClientConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	req := Request{
		Model:     "test",
		MaxTokens: 100,
		Stream:    true,
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("hi")}}},
	}

	ch, err := client.StreamMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("StreamMessage error: %v", err)
	}

	var eventTypes []StreamEventType
	for event := range ch {
		eventTypes = append(eventTypes, event.Type)
		if event.Type == EventDone {
			break
		}
	}

	if len(eventTypes) == 0 {
		t.Fatal("no events received")
	}
}

func TestNewAnthropicClientFromEnv(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "env-test-key")
	client, err := NewAnthropicClientFromEnv()
	if err != nil {
		t.Fatalf("NewAnthropicClientFromEnv error: %v", err)
	}
	if client.config.APIKey != "env-test-key" {
		t.Errorf("APIKey = %q, want %q", client.config.APIKey, "env-test-key")
	}
}

func TestNewAnthropicClientFromEnvMissingKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	t.Setenv("GLAW_API_KEY", "")
	_, err := NewAnthropicClientFromEnv()
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestBackoffDuration(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 30 * time.Second}, // capped
		{10, 30 * time.Second}, // still capped
	}
	for _, tt := range tests {
		got := backoffDuration(tt.attempt)
		if got != tt.want {
			t.Errorf("backoffDuration(%d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

func TestOpenAICompatClientSendMessage(t *testing.T) {
	oaiResp := `{
		"model": "grok-3",
		"choices": [{"message": {"role": "assistant", "content": "Hello from Grok"}, "finish_reason": "stop"}],
		"usage": {"total_tokens": 16}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("Path = %q, want to end with /chat/completions", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer xai-test-key" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, oaiResp)
	}))
	defer server.Close()

	client := NewOpenAICompatClient(ClientConfig{
		APIKey:  "xai-test-key",
		BaseURL: server.URL,
	})

	req := Request{
		Model:     "grok-3",
		MaxTokens: 100,
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("hi")}}},
	}

	resp, err := client.SendMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("SendMessage error: %v", err)
	}
	if len(resp.Content) != 1 || resp.Content[0].Text != "Hello from Grok" {
		t.Errorf("Content = %v", resp.Content)
	}
}
