package api

import (
	"encoding/json"
	"strings"
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

func TestContentBlockMarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		block   ContentBlock
		wantHas map[string]bool // keys that MUST appear in JSON output
		wantNot map[string]bool // keys that must NOT appear in JSON output
	}{
		{
			name:    "text block always includes text field",
			block:   NewTextBlock(""),
			wantHas: map[string]bool{`"text":""`: true, `"type":"text"`: true},
		},
		{
			name:    "text block with content includes text field",
			block:   NewTextBlock("hello"),
			wantHas: map[string]bool{`"text":"hello"`: true, `"type":"text"`: true},
		},
		{
			name:    "tool_use block always includes input field",
			block:   ContentBlock{Type: ContentToolUse, ID: "id1", Name: "bash", Input: nil},
			wantHas: map[string]bool{`"input":{}`: true, `"type":"tool_use"`: true},
		},
		{
			name:    "tool_use block preserves existing input",
			block:   NewToolUseBlock("id1", "bash", json.RawMessage(`{"command":"ls"}`)),
			wantHas: map[string]bool{`"command":"ls"`: true, `"type":"tool_use"`: true},
		},
		{
			name:    "tool_result block always includes content field even when empty",
			block:   NewToolResultBlock("toolu_123", "", false),
			wantHas: map[string]bool{`"content":""`: true, `"type":"tool_result"`: true},
		},
		{
			name:    "tool_result block with content",
			block:   NewToolResultBlock("toolu_123", "output here", false),
			wantHas: map[string]bool{`"content":"output here"`: true, `"type":"tool_result"`: true},
		},
		{
			name:    "tool_result error block always includes content field",
			block:   NewToolResultBlock("toolu_456", "", true),
			wantHas: map[string]bool{`"content":""`: true, `"is_error":true`: true, `"type":"tool_result"`: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.block)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}
			s := string(b)
			t.Logf("JSON: %s", s)

			for want := range tt.wantHas {
				if !strings.Contains(s, want) {
					t.Errorf("JSON output %q does not contain required %q", s, want)
				}
			}
			for notWant := range tt.wantNot {
				if strings.Contains(s, notWant) {
					t.Errorf("JSON output %q should not contain %q", s, notWant)
				}
			}
		})
	}
}

func TestToolResultEmptyContentNotOmitted(t *testing.T) {
	// This is the exact scenario that caused the bug:
	// A tool_result with empty content must still include "content":"" in JSON.
	// Previously, omitempty caused the content field to be omitted entirely,
	// which the Anthropic API treated as None/null, causing:
	// "sequence item 0: expected str instance, NoneType found"
	block := NewToolResultBlock("toolu_abc123", "", false)
	b, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	s := string(b)
	t.Logf("JSON: %s", s)

	if !strings.Contains(s, `"content":""`) {
		t.Errorf("tool_result with empty content MUST include \"content\":\"\" in JSON, got: %s", s)
	}
	if !strings.Contains(s, `"tool_use_id":"toolu_abc123"`) {
		t.Errorf("tool_result missing tool_use_id, got: %s", s)
	}
}

func TestToolUseNilInputNotOmitted(t *testing.T) {
	// Similarly, tool_use with nil input must include "input":{} in JSON.
	block := ContentBlock{Type: ContentToolUse, ID: "toolu_xyz", Name: "read_file", Input: nil}
	b, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	s := string(b)
	t.Logf("JSON: %s", s)

	if !strings.Contains(s, `"input":{}`) {
		t.Errorf("tool_use with nil input MUST include \"input\":{} in JSON, got: %s", s)
	}
}

func TestTextBlockEmptyNotOmitted(t *testing.T) {
	// Text blocks with empty text should still include "text":"" in JSON.
	block := NewTextBlock("")
	b, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	s := string(b)
	t.Logf("JSON: %s", s)

	if !strings.Contains(s, `"text":""`) {
		t.Errorf("text block with empty text MUST include \"text\":\"\" in JSON, got: %s", s)
	}
}

func TestMessageWithEmptyToolResult(t *testing.T) {
	// Simulate the exact API request that would fail:
	// A conversation with a tool result that has empty content.
	msgs := []Message{
		{Role: RoleUser, Content: []ContentBlock{NewTextBlock("run ls")}},
		{Role: RoleAssistant, Content: []ContentBlock{
			{Type: ContentToolUse, ID: "toolu_123", Name: "bash", Input: json.RawMessage(`{"command":"ls"}`)},
		}},
		{Role: RoleUser, Content: []ContentBlock{
			NewToolResultBlock("toolu_123", "", false), // empty content
		}},
	}

	b, err := json.Marshal(msgs)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	s := string(b)
	t.Logf("Messages JSON: %s", s)

	// Verify the tool_result message includes the content field
	if !strings.Contains(s, `"content":""`) {
		t.Errorf("tool_result in message must include \"content\":\"\" field, got: %s", s)
	}

	// Verify the tool_use message includes input
	if !strings.Contains(s, `"input":{"command":"ls"}`) {
		t.Errorf("tool_use in message must include input field, got: %s", s)
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
