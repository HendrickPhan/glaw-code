package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
		_, _ = w.Write(body)
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
			_, _ = w.Write([]byte(`{"error":"rate limit"}`))
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
		_, _ = w.Write(body)
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
		_, _ = w.Write([]byte(`{"error":"overloaded"}`))
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
		_, _ = w.Write([]byte(`{"error":"invalid key"}`))
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

func TestDetectProvider(t *testing.T) {
	tests := []struct {
		model string
		want  ProviderType
	}{
		{"claude-sonnet-4-6", ProviderAnthropic},
		{"claude-opus-4-6", ProviderAnthropic},
		{"grok-3", ProviderXAI},
		{"grok-3-mini", ProviderXAI},
		{"gpt-4o", ProviderOpenAI},
		{"gpt-4.1", ProviderOpenAI},
		{"gpt-4o-mini", ProviderOpenAI},
		{"o3", ProviderOpenAI},
		{"o3-mini", ProviderOpenAI},
		{"o4-mini", ProviderOpenAI},
		{"gemini-2.5-pro", ProviderGemini},
		{"gemini-2.0-flash", ProviderGemini},
		{"gemini-2.5-flash", ProviderGemini},
		{"ollama:llama3", ProviderOllama},
		{"ollama:qwen2.5-coder", ProviderOllama},
	}
	for _, tt := range tests {
		got := DetectProvider(tt.model)
		if got.Type != tt.want {
			t.Errorf("DetectProvider(%q) = %v, want %v", tt.model, got.Type, tt.want)
		}
	}
}

func TestDetectProviderOllamaEnvFallback(t *testing.T) {
	t.Setenv("OLLAMA_BASE_URL", "http://localhost:11434")
	got := DetectProvider("my-custom-model")
	if got.Type != ProviderOllama {
		t.Errorf("DetectProvider with OLLAMA_BASE_URL set = %v, want %v", got.Type, ProviderOllama)
	}
}

func TestResolveProviderOpenAI(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test-123")
	info := ProviderInfo{Type: ProviderOpenAI}
	resolved, err := info.Resolve("gpt-4o")
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if resolved.APIKey != "sk-test-123" {
		t.Errorf("APIKey = %q, want %q", resolved.APIKey, "sk-test-123")
	}
	if resolved.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("BaseURL = %q, want default OpenAI URL", resolved.BaseURL)
	}
}

func TestResolveProviderGemini(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "gemini-test-key")
	info := ProviderInfo{Type: ProviderGemini}
	resolved, err := info.Resolve("gemini-2.5-pro")
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if resolved.APIKey != "gemini-test-key" {
		t.Errorf("APIKey = %q, want %q", resolved.APIKey, "gemini-test-key")
	}
	if resolved.BaseURL != "https://generativelanguage.googleapis.com/v1beta/openai" {
		t.Errorf("BaseURL = %q", resolved.BaseURL)
	}
}

func TestResolveProviderGeminiGoogleKey(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "google-test-key")
	info := ProviderInfo{Type: ProviderGemini}
	resolved, err := info.Resolve("gemini-2.5-flash")
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if resolved.APIKey != "google-test-key" {
		t.Errorf("APIKey = %q, want %q", resolved.APIKey, "google-test-key")
	}
}

func TestResolveProviderOllama(t *testing.T) {
	info := ProviderInfo{Type: ProviderOllama}
	resolved, err := info.Resolve("ollama:llama3")
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if resolved.APIKey != "ollama" {
		t.Errorf("APIKey = %q, want %q", resolved.APIKey, "ollama")
	}
	if resolved.BaseURL != "http://localhost:11434/v1" {
		t.Errorf("BaseURL = %q", resolved.BaseURL)
	}
}

func TestResolveProviderMissingKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	info := ProviderInfo{Type: ProviderOpenAI}
	_, err := info.Resolve("gpt-4o")
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestNewProviderClientOpenAI(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-openai-test")
	client, err := NewProviderClient("gpt-4o")
	if err != nil {
		t.Fatalf("NewProviderClient error: %v", err)
	}
	oaiClient, ok := client.(*OpenAICompatClient)
	if !ok {
		t.Fatal("expected *OpenAICompatClient")
	}
	if oaiClient.config.APIKey != "sk-openai-test" {
		t.Errorf("APIKey = %q", oaiClient.config.APIKey)
	}
}

func TestNewProviderClientOllamaStripPrefix(t *testing.T) {
	client, err := NewProviderClient("ollama:llama3")
	if err != nil {
		t.Fatalf("NewProviderClient error: %v", err)
	}
	oaiClient, ok := client.(*OpenAICompatClient)
	if !ok {
		t.Fatal("expected *OpenAICompatClient")
	}
	if oaiClient.config.ModelAlias != "llama3" {
		t.Errorf("ModelAlias = %q, want %q", oaiClient.config.ModelAlias, "llama3")
	}
}

func TestOpenAICompatToolCallsResponse(t *testing.T) {
	oaiResp := `{
		"model": "gpt-4o",
		"choices": [{"message": {"role": "assistant", "content": null, "tool_calls": [{"id": "call_abc123", "type": "function", "function": {"name": "bash", "arguments": "{\"command\":\"ls -la\"}"}}]}, "finish_reason": "tool_calls"}],
		"usage": {"prompt_tokens": 50, "completion_tokens": 20, "total_tokens": 70}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, oaiResp)
	}))
	defer server.Close()

	client := NewOpenAICompatClient(ClientConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	req := Request{
		Model:     "gpt-4o",
		MaxTokens: 1024,
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("list files")}}},
		Tools: []ToolDefinition{
			{Name: "bash", Description: "Run a command", InputSchema: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`)},
		},
	}

	resp, err := client.SendMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("SendMessage error: %v", err)
	}
	if resp.StopReason != StopToolUse {
		t.Errorf("StopReason = %q, want %q", resp.StopReason, StopToolUse)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("Content length = %d, want 1", len(resp.Content))
	}
	block := resp.Content[0]
	if block.Type != ContentToolUse {
		t.Errorf("Content[0].Type = %q, want %q", block.Type, ContentToolUse)
	}
	if block.Name != "bash" {
		t.Errorf("Content[0].Name = %q, want %q", block.Name, "bash")
	}
	if block.ID != "call_abc123" {
		t.Errorf("Content[0].ID = %q, want %q", block.ID, "call_abc123")
	}
	if resp.Usage.InputTokens != 50 {
		t.Errorf("Usage.InputTokens = %d, want 50", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 20 {
		t.Errorf("Usage.OutputTokens = %d, want 20", resp.Usage.OutputTokens)
	}
}

func TestOpenAICompatToolResultConversion(t *testing.T) {
	oaiResp := `{
		"model": "gpt-4o",
		"choices": [{"message": {"role": "assistant", "content": "Done"}, "finish_reason": "stop"}],
		"usage": {"prompt_tokens": 30, "completion_tokens": 5, "total_tokens": 35}
	}`

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, oaiResp)
	}))
	defer server.Close()

	client := NewOpenAICompatClient(ClientConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	req := Request{
		Model:     "gpt-4o",
		MaxTokens: 1024,
		Messages: []Message{
			{Role: RoleUser, Content: []ContentBlock{NewTextBlock("list files")}},
			{Role: RoleAssistant, Content: []ContentBlock{NewToolUseBlock("call_1", "bash", json.RawMessage(`{"command":"ls"}`))}},
			{Role: RoleUser, Content: []ContentBlock{NewToolResultBlock("call_1", "file1.txt\nfile2.txt", false)}},
		},
	}

	_, err := client.SendMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("SendMessage error: %v", err)
	}

	// Verify the request was converted correctly
	var oaiReq oaiRequest
	if err := json.Unmarshal(capturedBody, &oaiReq); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}

	// Should have: system (none), user text, assistant with tool_calls, tool role with tool_call_id
	if len(oaiReq.Messages) < 3 {
		t.Fatalf("Messages length = %d, want >= 3", len(oaiReq.Messages))
	}

	// Assistant message should have tool_calls
	assistantMsg := oaiReq.Messages[1]
	if assistantMsg.Role != "assistant" {
		t.Errorf("Message[1] Role = %q, want 'assistant'", assistantMsg.Role)
	}
	if len(assistantMsg.ToolCalls) != 1 {
		t.Fatalf("Assistant ToolCalls length = %d, want 1", len(assistantMsg.ToolCalls))
	}
	if assistantMsg.ToolCalls[0].ID != "call_1" {
		t.Errorf("ToolCall ID = %q, want %q", assistantMsg.ToolCalls[0].ID, "call_1")
	}
	if assistantMsg.ToolCalls[0].Function.Name != "bash" {
		t.Errorf("ToolCall Name = %q, want %q", assistantMsg.ToolCalls[0].Function.Name, "bash")
	}

	// Tool result should be role:"tool" with tool_call_id
	toolMsg := oaiReq.Messages[2]
	if toolMsg.Role != "tool" {
		t.Errorf("Message[2] Role = %q, want 'tool'", toolMsg.Role)
	}
	if toolMsg.ToolCallID != "call_1" {
		t.Errorf("ToolCallID = %q, want %q", toolMsg.ToolCallID, "call_1")
	}
	if toolMsg.Content == nil || *toolMsg.Content != "file1.txt\nfile2.txt" {
		t.Errorf("Content = %v, want 'file1.txt\\nfile2.txt'", toolMsg.Content)
	}
}

func TestOpenAICompatStreaming(t *testing.T) {
	sseData := `data: {"choices":[{"delta":{"role":"assistant","content":""}}]}

data: {"choices":[{"delta":{"content":"Hello"}}]}

data: {"choices":[{"delta":{"content":" world"}}]}

data: {"choices":[{"delta":{},"finish_reason":"stop"}]}

data: [DONE]
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	client := NewOpenAICompatClient(ClientConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	req := Request{
		Model:    "gpt-4o",
		Messages: []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("hi")}}},
	}

	ch, err := client.StreamMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("StreamMessage error: %v", err)
	}

	var textDeltas []string
	var gotMessageDelta bool
	var gotDone bool
	for event := range ch {
		switch event.Type {
		case EventContentBlockDelta:
			if s, ok := event.Content.(string); ok {
				textDeltas = append(textDeltas, s)
			}
		case EventMessageDelta:
			gotMessageDelta = true
		case EventDone:
			gotDone = true
		}
	}

	if !gotDone {
		t.Error("expected EventDone")
	}
	if !gotMessageDelta {
		t.Error("expected EventMessageDelta with finish_reason")
	}
	if len(textDeltas) != 2 || textDeltas[0] != "Hello" || textDeltas[1] != " world" {
		t.Errorf("textDeltas = %v, want [Hello,  world]", textDeltas)
	}
}
