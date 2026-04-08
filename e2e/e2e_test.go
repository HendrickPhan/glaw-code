package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hieu-glaw/glaw-code/internal/api"
	"github.com/hieu-glaw/glaw-code/internal/server"
)

// TestE2EServerSessionLifecycle tests creating a session, sending a message, and retrieving it.
func TestE2EServerSessionLifecycle(t *testing.T) {
	srv := server.NewServer()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	client := ts.Client()

	// Step 1: Create a session
	resp, err := client.Post(ts.URL+"/sessions", "application/json", nil)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create session status %d: %s", resp.StatusCode, body)
	}

	var createResp server.CreateSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
			t.Fatalf("Decode: %v", err)
		}
	if createResp.SessionID == "" {
		t.Fatal("empty session ID")
	}
	t.Logf("Created session: %s", createResp.SessionID)

	// Step 2: List sessions
	resp, err = client.Get(ts.URL + "/sessions")
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	defer resp.Body.Close()

	var listResp server.ListSessionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
			t.Fatalf("Decode: %v", err)
		}
	if len(listResp.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(listResp.Sessions))
	}

	// Step 3: Get session details
	resp, err = client.Get(ts.URL + "/sessions/" + createResp.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	defer resp.Body.Close()

	var details server.SessionDetailsResponse
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
			t.Fatalf("Decode: %v", err)
		}
	if details.ID != createResp.SessionID {
		t.Errorf("session ID mismatch: %q != %q", details.ID, createResp.SessionID)
	}

	// Step 4: Send a message
	msgBody := strings.NewReader(`{"message":"Hello, world!"}`)
	resp, err = client.Post(ts.URL+"/sessions/"+createResp.SessionID+"/message", "application/json", msgBody)
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("send message status %d: %s", resp.StatusCode, body)
	}

	// Step 5: Verify session has message
	resp, err = client.Get(ts.URL + "/sessions/" + createResp.SessionID)
	if err != nil {
		t.Fatalf("get session after message: %v", err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
			t.Fatalf("Decode: %v", err)
		}
	if details.Session == nil {
		t.Fatal("session should not be nil")
	}
}

// TestE2ESSEStream tests SSE streaming from the server.
func TestE2ESSEStream(t *testing.T) {
	srv := server.NewServer()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Create session
	resp, err := http.Post(ts.URL+"/sessions", "application/json", nil)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	var createResp server.CreateSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
			t.Fatalf("Decode: %v", err)
		}
	resp.Body.Close()

	// Connect to SSE stream with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		ts.URL+"/sessions/"+createResp.SessionID+"/events", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("connect SSE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("SSE status = %d", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	// Read initial snapshot event
	scanner := NewSSEScanner(resp.Body)
	if !scanner.Scan() {
		t.Fatal("expected SSE event")
	}
	event := scanner.Event()
	if event == "" {
		t.Error("empty SSE event type")
	}
}

// TestE2EMultipleSessions tests creating and listing multiple sessions.
func TestE2EMultipleSessions(t *testing.T) {
	srv := server.NewServer()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Create 5 sessions
	ids := make(map[string]bool)
	for i := 0; i < 5; i++ {
		resp, err := http.Post(ts.URL+"/sessions", "application/json", nil)
		if err != nil {
			t.Fatalf("create session %d: %v", i, err)
		}
		var createResp server.CreateSessionResponse
		if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
				t.Fatalf("decode session %d: %v", i, err)
			}
		resp.Body.Close()
		ids[createResp.SessionID] = true
	}

	// List sessions
	resp, err := http.Get(ts.URL + "/sessions")
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	var listResp server.ListSessionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
			t.Fatalf("Decode: %v", err)
		}
	resp.Body.Close()

	if len(listResp.Sessions) != 5 {
		t.Errorf("expected 5 sessions, got %d", len(listResp.Sessions))
	}

	for _, s := range listResp.Sessions {
		if !ids[s.ID] {
			t.Errorf("unexpected session ID: %s", s.ID)
		}
	}
}

// TestE2EAPIStreaming tests the API SSE parser with a real SSE stream.
func TestE2EAPIStreaming(t *testing.T) {
	sseData := strings.NewReader(
		"event: message_start\n" +
			"data: {\"type\":\"message_start\"}\n\n" +
			"event: content_block_delta\n" +
			"data: {\"type\":\"content_block_delta\"}\n\n" +
			"event: message_stop\n" +
			"data: {\"type\":\"message_stop\"}\n\n")

	events, err := api.ParseSSEStream(sseData)
	if err != nil {
		t.Fatalf("ParseSSEStream error: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	wantEvents := []string{"message_start", "content_block_delta", "message_stop"}
	for i, want := range wantEvents {
		if events[i].Event != want {
			t.Errorf("events[%d].Event = %q, want %q", i, events[i].Event, want)
		}
	}
}

// TestE2EAPIRequest tests building and serializing API requests.
func TestE2EAPIRequest(t *testing.T) {
	req := api.Request{
		Model:     "openrouter:nvidia/nemotron-3-super-120b-a12b:free",
		MaxTokens: 4096,
		Stream:    true,
		System:    "You are a helpful assistant.",
		Messages: []api.Message{
			{
				Role:    api.RoleUser,
				Content: []api.ContentBlock{api.NewTextBlock("What is 2+2?")},
			},
		},
		Tools: []api.ToolDefinition{
			{
				Name:        "calculator",
				Description: "Performs calculations",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"expression":{"type":"string"}}}`),
			},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if parsed["model"] != "openrouter:nvidia/nemotron-3-super-120b-a12b:free" {
		t.Errorf("model = %v", parsed["model"])
	}
	if parsed["stream"] != true {
		t.Error("stream should be true")
	}
	if parsed["system"] != "You are a helpful assistant." {
		t.Errorf("system = %v", parsed["system"])
	}

	msgs := parsed["messages"].([]interface{})
	if len(msgs) != 1 {
		t.Errorf("messages count = %d", len(msgs))
	}

	tools := parsed["tools"].([]interface{})
	if len(tools) != 1 {
		t.Errorf("tools count = %d", len(tools))
	}
}

// SSEScanner is a helper for reading SSE events in tests.
type SSEScanner struct {
	_ interface {
		Scan() bool
		Text() string
	}
	buf *strings.Builder
	r   io.Reader
}

func NewSSEScanner(r io.Reader) *SSEScanner {
	return &SSEScanner{r: r, buf: &strings.Builder{}}
}

func (s *SSEScanner) Scan() bool {
	buf := make([]byte, 4096)
	n, err := s.r.Read(buf)
	if err != nil || n == 0 {
		return false
	}
	s.buf.Write(buf[:n])
	return true
}

func (s *SSEScanner) Event() string {
	data := s.buf.String()
	for _, line := range strings.Split(data, "\n") {
		if strings.HasPrefix(line, "event: ") {
			return strings.TrimPrefix(line, "event: ")
		}
	}
	return ""
}

// TestE2EProviderRouting tests the provider routing logic.
func TestE2EProviderRouting(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		wantErr  bool
		setupEnv func()
	}{
		{
			name:    "anthropic from env",
			model:   "claude-sonnet-4-6",
			wantErr: false,
			setupEnv: func() {
				t.Setenv("ANTHROPIC_API_KEY", "test-key")
				t.Setenv("OPENROUTER_API_KEY", "")
			},
		},
		{
			name:    "grok requires XAI_API_KEY",
			model:   "grok-3",
			wantErr: true,
			setupEnv: func() {
				t.Setenv("XAI_API_KEY", "")
			},
		},
		{
			name:    "grok with XAI_API_KEY",
			model:   "grok-3",
			wantErr: false,
			setupEnv: func() {
				t.Setenv("XAI_API_KEY", "xai-test-key")
			},
		},
		{
			name:    "missing all keys",
			model:   "claude-sonnet-4-6",
			wantErr: true,
			setupEnv: func() {
				t.Setenv("ANTHROPIC_API_KEY", "")
				t.Setenv("GLAW_API_KEY", "")
				t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv()
			_, err := api.NewProviderClient(tt.model)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewProviderClient() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestE2EAPIRoundTrip tests a full API round trip with a mock server.
func TestE2EAPIRoundTrip(t *testing.T) {
	response := api.Response{
		ID:         "msg_e2e_123",
		StopReason: api.StopEndTurn,
		Usage:      api.Usage{InputTokens: 10, OutputTokens: 20},
		Content:    []api.ContentBlock{api.NewTextBlock("The answer is 4.")},
	}
	body, _ := json.Marshal(response)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("missing API key header")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("x-request-id", "req_e2e_456")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer mockServer.Close()

	client := api.NewAnthropicClient(api.ClientConfig{
		APIKey:  "test-key",
		BaseURL: mockServer.URL,
	})

	req := api.Request{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 1024,
		Messages: []api.Message{
			{Role: api.RoleUser, Content: []api.ContentBlock{api.NewTextBlock("What is 2+2?")}},
		},
	}

	resp, err := client.SendMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("SendMessage error: %v", err)
	}

	if resp.ID != "msg_e2e_123" {
		t.Errorf("ID = %q", resp.ID)
	}
	if resp.RequestID != "req_e2e_456" {
		t.Errorf("RequestID = %q", resp.RequestID)
	}
	if resp.StopReason != api.StopEndTurn {
		t.Errorf("StopReason = %q", resp.StopReason)
	}
	if len(resp.Content) != 1 || resp.Content[0].Text != "The answer is 4." {
		t.Errorf("Content = %v", resp.Content)
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 20 {
		t.Errorf("Usage = %+v", resp.Usage)
	}
}
