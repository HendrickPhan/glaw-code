package entity

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	api "github.com/hieu-glaw/glaw-code/internal/api"
	permentity "github.com/hieu-glaw/glaw-code/internal/modules/permission/domain/entity"
	permservice "github.com/hieu-glaw/glaw-code/internal/modules/permission/domain/service"
	sessionentity "github.com/hieu-glaw/glaw-code/internal/modules/session/domain/entity"
)

// =====================================================================
// Mock Streaming Client
// =====================================================================

// mockStreamClient implements api.ProviderClient for streaming tests.
type mockStreamClient struct {
	events   []api.StreamEvent
	sendErr  error
	mu       sync.Mutex
	sentReqs []api.Request
}

func (m *mockStreamClient) SendMessage(ctx context.Context, req api.Request) (*api.Response, error) {
	return nil, nil
}

func (m *mockStreamClient) StreamMessage(ctx context.Context, req api.Request) (<-chan api.StreamEvent, error) {
	m.mu.Lock()
	m.sentReqs = append(m.sentReqs, req)
	m.mu.Unlock()

	if m.sendErr != nil {
		return nil, m.sendErr
	}

	ch := make(chan api.StreamEvent, len(m.events)+2)
	for _, e := range m.events {
		ch <- e
	}
	ch <- api.StreamEvent{Type: api.EventDone}
	close(ch)
	return ch, nil
}

func newStreamTestRuntime(client api.ProviderClient) *ConversationRuntime {
	return NewConversationRuntime(
		client,
		DefaultConfig(),
		sessionentity.NewSession(),
		permservice.NewPermissionManager(permentity.PermDangerFullAccess, "/tmp"),
		nil,
	)
}

// =====================================================================
// extractTextFromAnthropicDelta tests
// =====================================================================

func TestExtractTextFromAnthropicDelta_TextDelta(t *testing.T) {
	raw := []byte(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}`)
	got := extractTextFromAnthropicDelta(raw)
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestExtractTextFromAnthropicDelta_InputJSONDelta(t *testing.T) {
	raw := []byte(`{"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"{\"cmd\":\"ls\"}"}}`)
	got := extractTextFromAnthropicDelta(raw)
	if got != "" {
		t.Errorf("expected empty string for input_json_delta, got %q", got)
	}
}

func TestExtractTextFromAnthropicDelta_InvalidJSON(t *testing.T) {
	got := extractTextFromAnthropicDelta([]byte(`not json`))
	if got != "" {
		t.Errorf("expected empty string for invalid JSON, got %q", got)
	}
}

func TestExtractTextFromAnthropicDelta_EmptyText(t *testing.T) {
	raw := []byte(`{"type":"content_block_delta","delta":{"type":"text_delta","text":""}}`)
	got := extractTextFromAnthropicDelta(raw)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// =====================================================================
// AccumulateStream tests
// =====================================================================

func TestAccumulateStream_OpenAI_TextOnly(t *testing.T) {
	rt := newStreamTestRuntime(&mockStreamClient{})

	events := []api.StreamEvent{
		{Type: api.EventMessageStart, Content: api.Usage{InputTokens: 10}},
		{Type: api.EventContentBlockDelta, Content: "Hello "},
		{Type: api.EventContentBlockDelta, Content: "world!"},
		{Type: api.EventMessageDelta, Content: api.StopEndTurn},
		{Type: api.EventMessageStop, Content: &api.Response{
			Content:    []api.ContentBlock{api.NewTextBlock("Hello world!")},
			StopReason: api.StopEndTurn,
			Usage:      api.Usage{InputTokens: 10, OutputTokens: 5},
		}},
	}

	ctx := context.Background()
	ch := make(chan api.StreamEvent, len(events)+1)
	for _, e := range events {
		ch <- e
	}
	ch <- api.StreamEvent{Type: api.EventDone}
	close(ch)

	resp, err := rt.AccumulateStream(ctx, ch)
	if err != nil {
		t.Fatalf("AccumulateStream error: %v", err)
	}

	merged := resp.MergeText()
	if merged != "Hello world!" {
		t.Errorf("MergeText() = %q, want %q", merged, "Hello world!")
	}

	if len(resp.Content) != 1 {
		t.Fatalf("Content length = %d, want 1", len(resp.Content))
	}
	if resp.Content[0].Text != "Hello world!" {
		t.Errorf("Content[0].Text = %q, want %q", resp.Content[0].Text, "Hello world!")
	}

	if resp.StopReason != api.StopEndTurn {
		t.Errorf("StopReason = %q, want %q", resp.StopReason, api.StopEndTurn)
	}

	if resp.Usage.OutputTokens != 5 {
		t.Errorf("Usage.OutputTokens = %d, want 5", resp.Usage.OutputTokens)
	}
}

func TestAccumulateStream_Anthropic_TextDeltas(t *testing.T) {
	rt := newStreamTestRuntime(&mockStreamClient{})

	events := []api.StreamEvent{
		{Type: api.EventMessageStart, Content: api.Usage{InputTokens: 20}},
		{Type: api.EventContentBlockDelta, Content: []byte(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"Foo"}}`)},
		{Type: api.EventContentBlockDelta, Content: []byte(`{"type":"content_block_delta","delta":{"type":"text_delta","text":" bar"}}`)},
		{Type: api.EventMessageDelta, Content: api.StopEndTurn},
		{Type: api.EventMessageStop, Content: &api.Response{
			Content:    []api.ContentBlock{api.NewTextBlock("Foo bar")},
			StopReason: api.StopEndTurn,
			Usage:      api.Usage{InputTokens: 20, OutputTokens: 8},
		}},
	}

	ctx := context.Background()
	ch := make(chan api.StreamEvent, len(events)+1)
	for _, e := range events {
		ch <- e
	}
	ch <- api.StreamEvent{Type: api.EventDone}
	close(ch)

	resp, err := rt.AccumulateStream(ctx, ch)
	if err != nil {
		t.Fatalf("AccumulateStream error: %v", err)
	}

	merged := resp.MergeText()
	if merged != "Foo bar" {
		t.Errorf("MergeText() = %q, want %q", merged, "Foo bar")
	}
}

func TestAccumulateStream_ToolUseBlocks(t *testing.T) {
	rt := newStreamTestRuntime(&mockStreamClient{})

	toolCall := api.NewToolUseBlock("toolu_123", "bash", json.RawMessage(`{"command":"ls"}`))

	events := []api.StreamEvent{
		{Type: api.EventMessageStart, Content: api.Usage{InputTokens: 10}},
		{Type: api.EventContentBlockDelta, Content: "I'll list files.\n"},
		{Type: api.EventMessageDelta, Content: api.StopToolUse},
		{Type: api.EventMessageStop, Content: &api.Response{
			Content:    []api.ContentBlock{api.NewTextBlock("I'll list files.\n"), toolCall},
			StopReason: api.StopToolUse,
			Usage:      api.Usage{InputTokens: 10, OutputTokens: 20},
		}},
	}

	ctx := context.Background()
	ch := make(chan api.StreamEvent, len(events)+1)
	for _, e := range events {
		ch <- e
	}
	ch <- api.StreamEvent{Type: api.EventDone}
	close(ch)

	resp, err := rt.AccumulateStream(ctx, ch)
	if err != nil {
		t.Fatalf("AccumulateStream error: %v", err)
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("ToolCalls length = %d, want 1", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "bash" {
		t.Errorf("ToolCalls[0].Name = %q, want %q", resp.ToolCalls[0].Name, "bash")
	}
	if resp.StopReason != api.StopToolUse {
		t.Errorf("StopReason = %q, want %q", resp.StopReason, api.StopToolUse)
	}
}

func TestAccumulateStream_ContextCancelled(t *testing.T) {
	rt := newStreamTestRuntime(&mockStreamClient{})

	// Use a context that's already cancelled — the key is that we check
	// ctx.Err() when we get an error event
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ch := make(chan api.StreamEvent, 1)
	ch <- api.StreamEvent{Type: api.EventError, Error: context.Canceled}
	close(ch)

	_, err := rt.AccumulateStream(ctx, ch)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestAccumulateStream_StreamError(t *testing.T) {
	rt := newStreamTestRuntime(&mockStreamClient{})

	ctx := context.Background()
	ch := make(chan api.StreamEvent, 1)
	ch <- api.StreamEvent{Type: api.EventError, Error: context.DeadlineExceeded}
	close(ch)

	resp, err := rt.AccumulateStream(ctx, ch)
	if err == nil {
		t.Fatal("expected error from stream")
	}
	if resp == nil {
		t.Fatal("expected non-nil response even on error")
	}
}

func TestAccumulateStream_FallbackNoMessageStop(t *testing.T) {
	rt := newStreamTestRuntime(&mockStreamClient{})

	ctx := context.Background()
	ch := make(chan api.StreamEvent, 3)
	ch <- api.StreamEvent{Type: api.EventMessageStart, Content: api.Usage{}}
	ch <- api.StreamEvent{Type: api.EventContentBlockDelta, Content: "fallback"}
	ch <- api.StreamEvent{Type: api.EventDone}
	close(ch)

	resp, err := rt.AccumulateStream(ctx, ch)
	if err != nil {
		t.Fatalf("AccumulateStream error: %v", err)
	}

	// Fallback builds one ContentBlock per TextBlock entry
	if len(resp.Content) != 1 {
		t.Fatalf("Content length = %d, want 1", len(resp.Content))
	}
	merged := resp.MergeText()
	if merged != "fallback" {
		t.Errorf("MergeText() = %q, want %q", merged, "fallback")
	}
}

func TestAccumulateStream_FallbackWithMessageStopNoResponse(t *testing.T) {
	rt := newStreamTestRuntime(&mockStreamClient{})

	ctx := context.Background()
	ch := make(chan api.StreamEvent, 4)
	ch <- api.StreamEvent{Type: api.EventMessageStart, Content: api.Usage{}}
	ch <- api.StreamEvent{Type: api.EventContentBlockDelta, Content: "text1"}
	ch <- api.StreamEvent{Type: api.EventContentBlockDelta, Content: " text2"}
	ch <- api.StreamEvent{Type: api.EventMessageStop, Content: "not a response"}
	close(ch)

	resp, err := rt.AccumulateStream(ctx, ch)
	if err != nil {
		t.Fatalf("AccumulateStream error: %v", err)
	}

	merged := resp.MergeText()
	if merged != "text1 text2" {
		t.Errorf("MergeText() = %q, want %q", merged, "text1 text2")
	}

	if len(resp.Content) != 2 {
		t.Fatalf("Content length = %d, want 2 (one per text block)", len(resp.Content))
	}
}

// =====================================================================
// ConsumeStream tests
// =====================================================================

func TestConsumeStream_TextCallbackCalled(t *testing.T) {
	rt := newStreamTestRuntime(&mockStreamClient{})

	var mu sync.Mutex
	var received []string

	callback := func(text string) {
		mu.Lock()
		received = append(received, text)
		mu.Unlock()
	}

	events := []api.StreamEvent{
		{Type: api.EventMessageStart, Content: api.Usage{}},
		{Type: api.EventContentBlockDelta, Content: "a"},
		{Type: api.EventContentBlockDelta, Content: "b"},
		{Type: api.EventContentBlockDelta, Content: "c"},
		{Type: api.EventMessageDelta, Content: api.StopEndTurn},
		{Type: api.EventMessageStop, Content: &api.Response{
			Content:    []api.ContentBlock{api.NewTextBlock("abc")},
			StopReason: api.StopEndTurn,
			Usage:      api.Usage{InputTokens: 5, OutputTokens: 3},
		}},
	}

	ctx := context.Background()
	ch := make(chan api.StreamEvent, len(events)+1)
	for _, e := range events {
		ch <- e
	}
	ch <- api.StreamEvent{Type: api.EventDone}
	close(ch)

	resp, err := rt.ConsumeStream(ctx, ch, callback)
	if err != nil {
		t.Fatalf("ConsumeStream error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 3 {
		t.Errorf("callback called %d times, want 3", len(received))
	}
	if strings.Join(received, "") != "abc" {
		t.Errorf("callback received %q, want %q", strings.Join(received, ""), "abc")
	}
	if resp.Usage.OutputTokens != 3 {
		t.Errorf("Usage.OutputTokens = %d, want 3", resp.Usage.OutputTokens)
	}
}

func TestConsumeStream_NilCallback(t *testing.T) {
	rt := newStreamTestRuntime(&mockStreamClient{})

	events := []api.StreamEvent{
		{Type: api.EventMessageStart, Content: api.Usage{}},
		{Type: api.EventContentBlockDelta, Content: "hello"},
		{Type: api.EventMessageStop, Content: &api.Response{
			Content:    []api.ContentBlock{api.NewTextBlock("hello")},
			StopReason: api.StopEndTurn,
			Usage:      api.Usage{InputTokens: 5, OutputTokens: 3},
		}},
	}

	ctx := context.Background()
	ch := make(chan api.StreamEvent, len(events)+1)
	for _, e := range events {
		ch <- e
	}
	ch <- api.StreamEvent{Type: api.EventDone}
	close(ch)

	// Should not panic with nil callback
	_, err := rt.ConsumeStream(ctx, ch, nil)
	if err != nil {
		t.Fatalf("ConsumeStream error: %v", err)
	}
}

func TestConsumeStream_AnthropicDeltaWithCallback(t *testing.T) {
	rt := newStreamTestRuntime(&mockStreamClient{})

	var mu sync.Mutex
	var received []string

	events := []api.StreamEvent{
		{Type: api.EventMessageStart, Content: api.Usage{}},
		{Type: api.EventContentBlockDelta, Content: []byte(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"Anthropic"}}`)},
		{Type: api.EventContentBlockDelta, Content: []byte(`{"type":"content_block_delta","delta":{"type":"text_delta","text":" streaming"}}`)},
		{Type: api.EventMessageStop, Content: &api.Response{
			Content:    []api.ContentBlock{api.NewTextBlock("Anthropic streaming")},
			StopReason: api.StopEndTurn,
			Usage:      api.Usage{InputTokens: 10, OutputTokens: 10},
		}},
	}

	ctx := context.Background()
	ch := make(chan api.StreamEvent, len(events)+1)
	for _, e := range events {
		ch <- e
	}
	ch <- api.StreamEvent{Type: api.EventDone}
	close(ch)

	resp, err := rt.ConsumeStream(ctx, ch, func(text string) {
		mu.Lock()
		received = append(received, text)
		mu.Unlock()
	})

	if err != nil {
		t.Fatalf("ConsumeStream error: %v", err)
	}
	_ = resp // verify it's not nil
	mu.Lock()
	if strings.Join(received, "") != "Anthropic streaming" {
		t.Errorf("callback received %q, want %q", strings.Join(received, ""), "Anthropic streaming")
	}
	mu.Unlock()
}

// =====================================================================
// StreamResponse tests
// =====================================================================

func TestStreamResponse_MergeText(t *testing.T) {
	sr := &StreamResponse{
		TextBlocks: []string{"Hello", " ", "world", "!"},
	}
	merged := sr.MergeText()
	if merged != "Hello world!" {
		t.Errorf("MergeText() = %q, want %q", merged, "Hello world!")
	}
}

func TestStreamResponse_MergeTextEmpty(t *testing.T) {
	sr := &StreamResponse{}
	if sr.MergeText() != "" {
		t.Errorf("MergeText() = %q, want empty string", sr.MergeText())
	}
}

func TestStreamResponse_MergeTextMultipleChunks(t *testing.T) {
	sr := &StreamResponse{
		TextBlocks: []string{"chunk1", "chunk2", "chunk3"},
		ToolCalls:  []api.ContentBlock{},
	}
	if sr.MergeText() != "chunk1chunk2chunk3" {
		t.Errorf("MergeText() = %q, want %q", sr.MergeText(), "chunk1chunk2chunk3")
	}
}

func TestRunLoopStream_TextOnly(t *testing.T) {
	client := &mockStreamClient{
		events: []api.StreamEvent{
			{Type: api.EventMessageStart, Content: api.Usage{InputTokens: 10}},
			{Type: api.EventContentBlockDelta, Content: "Hi"},
			{Type: api.EventContentBlockDelta, Content: " there!"},
			{Type: api.EventMessageDelta, Content: api.StopEndTurn},
			{Type: api.EventMessageStop, Content: &api.Response{
				Content:    []api.ContentBlock{api.NewTextBlock("Hi there!")},
				StopReason: api.StopEndTurn,
				Usage:      api.Usage{InputTokens: 10, OutputTokens: 8},
			}},
		},
	}

	rt := newStreamTestRuntime(client)
	rt.Session.AddUserMessageFromText("hello")

	var mu sync.Mutex
	var deltas []string

	err := rt.RunLoopStream(context.Background(), func(delta string) {
		mu.Lock()
		deltas = append(deltas, delta)
		mu.Unlock()
	})

	if err != nil {
		t.Fatalf("RunLoopStream error: %v", err)
	}

	mu.Lock()
	if strings.Join(deltas, "") != "Hi there!" {
		t.Errorf("received deltas = %q, want %q", strings.Join(deltas, ""), "Hi there!")
	}
	mu.Unlock()

	if rt.Usage.Cumulative.OutputTokens != 8 {
		t.Errorf("OutputTokens = %d, want 8", rt.Usage.Cumulative.OutputTokens)
	}
}

func TestRunLoopStream_ContextCancelled(t *testing.T) {
	blockingCh := make(chan api.StreamEvent)

	// Use a special blocking client
	blockingClient := &blockingStreamClient{ch: blockingCh}

	rt := newStreamTestRuntime(blockingClient)
	rt.Session.AddUserMessageFromText("hello")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- rt.RunLoopStream(ctx, func(delta string) {})
	}()

	select {
	case err := <-doneCh:
		if err == nil {
			t.Fatal("expected error from timeout")
		}
		if !IsActionCancelled(err) {
			t.Errorf("expected ActionCancelledError, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunLoopStream did not return within 2s")
	}
}

// blockingStreamClient is a mock that returns a channel that blocks forever.
type blockingStreamClient struct {
	ch <-chan api.StreamEvent
}

func (b *blockingStreamClient) SendMessage(ctx context.Context, req api.Request) (*api.Response, error) {
	return nil, nil
}

func (b *blockingStreamClient) StreamMessage(ctx context.Context, req api.Request) (<-chan api.StreamEvent, error) {
	return b.ch, nil
}

func TestRunLoopStream_ToolUseWithExecution(t *testing.T) {
	toolCall := api.NewToolUseBlock("toolu_1", "bash", json.RawMessage(`{"command":"echo hi"}`))

	// Provide two sets of events: first turn (with tool call) and second turn (after tool)
	client := &mockStreamClient{
		events: []api.StreamEvent{
			// First turn
			{Type: api.EventMessageStart, Content: api.Usage{InputTokens: 10}},
			{Type: api.EventContentBlockDelta, Content: "Running...\n"},
			{Type: api.EventMessageDelta, Content: api.StopToolUse},
			{Type: api.EventMessageStop, Content: &api.Response{
				Content:    []api.ContentBlock{api.NewTextBlock("Running...\n"), toolCall},
				StopReason: api.StopToolUse,
				Usage:      api.Usage{InputTokens: 10, OutputTokens: 15},
			}},
			// Second turn (after tool execution loops back)
			{Type: api.EventMessageStart, Content: api.Usage{InputTokens: 100}},
			{Type: api.EventContentBlockDelta, Content: "Done!"},
			{Type: api.EventMessageDelta, Content: api.StopEndTurn},
			{Type: api.EventMessageStop, Content: &api.Response{
				Content:    []api.ContentBlock{api.NewTextBlock("Done!")},
				StopReason: api.StopEndTurn,
				Usage:      api.Usage{InputTokens: 100, OutputTokens: 5},
			}},
		},
	}

	// Use the existing mockToolExec from snapshot_test.go (same package)
	mockExec := &mockToolExec{
		results: map[string]*ToolOutput{
			"bash": {Content: "hi\n", IsError: false},
		},
	}

	rt := NewConversationRuntime(
		client,
		DefaultConfig(),
		sessionentity.NewSession(),
		permservice.NewPermissionManager(permentity.PermDangerFullAccess, "/tmp"),
		mockExec,
	)
	rt.Session.AddUserMessageFromText("run echo")

	var mu sync.Mutex
	var deltas []string

	err := rt.RunLoopStream(context.Background(), func(delta string) {
		mu.Lock()
		deltas = append(deltas, delta)
		mu.Unlock()
	})

	if err != nil {
		t.Fatalf("RunLoopStream error: %v", err)
	}

	mu.Lock()
	all := strings.Join(deltas, "")
	mu.Unlock()

	if !strings.Contains(all, "Running...") {
		t.Errorf("expected 'Running...' in deltas, got %q", all)
	}
	if !strings.Contains(all, "Done!") {
		t.Errorf("expected 'Done!' in deltas, got %q", all)
	}
}
