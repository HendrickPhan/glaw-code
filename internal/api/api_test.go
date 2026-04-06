package api

import (
	"testing"
)

func TestContentBlockTypes(t *testing.T) {
	tests := []struct {
		name    string
		block   ContentBlock
		want    ContentBlockType
	}{
		{"text block", NewTextBlock("hello"), ContentText},
		{"tool use block", NewToolUseBlock("id1", "bash", nil), ContentToolUse},
		{"tool result block", NewToolResultBlock("id1", "output", false), ContentToolResult},
		{"error result block", NewToolResultBlock("id1", "error", true), ContentToolResult},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.block.Type != tt.want {
				t.Errorf("ContentBlock.Type = %v, want %v", tt.block.Type, tt.want)
			}
		})
	}
}

func TestNewTextBlock(t *testing.T) {
	b := NewTextBlock("hello world")
	if b.Text != "hello world" {
		t.Errorf("Text = %q, want %q", b.Text, "hello world")
	}
	if b.Type != ContentText {
		t.Errorf("Type = %v, want %v", b.Type, ContentText)
	}
}

func TestNewToolUseBlock(t *testing.T) {
	input := []byte(`{"command":"ls"}`)
	b := NewToolUseBlock("call_123", "bash", input)
	if b.ID != "call_123" {
		t.Errorf("ID = %q, want %q", b.ID, "call_123")
	}
	if b.Name != "bash" {
		t.Errorf("Name = %q, want %q", b.Name, "bash")
	}
	if string(b.Input) != `{"command":"ls"}` {
		t.Errorf("Input = %q, want %q", string(b.Input), `{"command":"ls"}`)
	}
}

func TestNewToolUseBlockNilInput(t *testing.T) {
	// Nil input should default to empty JSON object {} to prevent
	// "expected str instance, NoneType found" API errors
	b := NewToolUseBlock("call_123", "bash", nil)
	if string(b.Input) != `{}` {
		t.Errorf("Input = %q, want %q for nil input", string(b.Input), `{}`)
	}
}

func TestNewToolResultBlock(t *testing.T) {
	b := NewToolResultBlock("call_123", "file contents", false)
	if b.ToolUseID != "call_123" {
		t.Errorf("ToolUseID = %q, want %q", b.ToolUseID, "call_123")
	}
	if b.IsError {
		t.Error("IsError should be false")
	}

	bErr := NewToolResultBlock("call_456", "command failed", true)
	if !bErr.IsError {
		t.Error("IsError should be true")
	}
}

func TestMessageRoles(t *testing.T) {
	if RoleUser != "user" {
		t.Errorf("RoleUser = %q, want %q", RoleUser, "user")
	}
	if RoleAssistant != "assistant" {
		t.Errorf("RoleAssistant = %q, want %q", RoleAssistant, "assistant")
	}
}

func TestStopReasons(t *testing.T) {
	reasons := map[StopReason]string{
		StopEndTurn:   "end_turn",
		StopToolUse:   "tool_use",
		StopMaxTokens: "max_tokens",
		StopSequence:  "stop_sequence",
	}
	for reason, want := range reasons {
		if string(reason) != want {
			t.Errorf("StopReason = %q, want %q", reason, want)
		}
	}
}

func TestRequest(t *testing.T) {
	req := Request{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 8096,
		Stream:    true,
		Messages: []Message{
			{Role: RoleUser, Content: []ContentBlock{NewTextBlock("hello")}},
		},
	}
	if req.Model != "claude-sonnet-4-6" {
		t.Errorf("Model = %q", req.Model)
	}
	if !req.Stream {
		t.Error("Stream should be true")
	}
	if len(req.Messages) != 1 {
		t.Errorf("Messages count = %d, want 1", len(req.Messages))
	}
}
