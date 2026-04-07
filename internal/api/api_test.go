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

func TestToolResultMustNotIncludeID(t *testing.T) {
	// This is the exact bug fix: tool_result blocks must NOT include an "id" field.
	// Previously, the MarshalJSON method unconditionally copied the ID field
	// into the JSON output for all block types. The Anthropic API server crashes
	// with a 500 error when it encounters an "id" field on a tool_result block:
	//   "'ClaudeContentBlockToolResult' object has no attribute 'id'"
	//
	// This test verifies that tool_result blocks never emit an "id" field,
	// even when the ContentBlock.ID field is set to a non-empty value.
	block := ContentBlock{
		Type:      ContentToolResult,
		ID:        "should_not_appear", // This ID should NOT be in the JSON output
		ToolUseID: "toolu_abc123",
		Content:   "result output",
		IsError:   false,
	}

	b, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	s := string(b)
	t.Logf("JSON: %s", s)

	// Must NOT contain "id" field
	if strings.Contains(s, `"id"`) {
		t.Errorf("tool_result block MUST NOT include \"id\" field in JSON, got: %s", s)
	}

	// Must contain expected fields
	if !strings.Contains(s, `"tool_use_id":"toolu_abc123"`) {
		t.Errorf("tool_result missing tool_use_id, got: %s", s)
	}
	if !strings.Contains(s, `"content":"result output"`) {
		t.Errorf("tool_result missing content, got: %s", s)
	}
	if !strings.Contains(s, `"type":"tool_result"`) {
		t.Errorf("tool_result missing type, got: %s", s)
	}
}

func TestToolUseStillIncludesID(t *testing.T) {
	// Verify that tool_use blocks still include the "id" field after the fix.
	block := ContentBlock{
		Type:  ContentToolUse,
		ID:    "toolu_123",
		Name:  "bash",
		Input: json.RawMessage(`{"command":"ls"}`),
	}

	b, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	s := string(b)
	t.Logf("JSON: %s", s)

	if !strings.Contains(s, `"id":"toolu_123"`) {
		t.Errorf("tool_use block MUST include \"id\" field in JSON, got: %s", s)
	}
}

func TestTextBlockWithIDIncludesID(t *testing.T) {
	// Verify that text blocks with an ID still include it.
	block := ContentBlock{
		Type: ContentText,
		ID:   "msg_123",
		Text: "hello",
	}

	b, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	s := string(b)
	t.Logf("JSON: %s", s)

	if !strings.Contains(s, `"id":"msg_123"`) {
		t.Errorf("text block with ID should include \"id\" field, got: %s", s)
	}
}

func TestTextBlockEmptyIDNotIncluded(t *testing.T) {
	// Text blocks with empty ID should not include "id" field.
	block := NewTextBlock("hello")

	b, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	s := string(b)
	t.Logf("JSON: %s", s)

	if strings.Contains(s, `"id"`) {
		t.Errorf("text block with empty ID should not include \"id\" field, got: %s", s)
	}
}

// TestAnthropicAPIFieldContract validates that every ContentBlock type
// produces exactly the fields the Anthropic Messages API expects.
// This is the comprehensive test to prevent any "object has no attribute"
// style server-side 500 errors.
//
// Anthropic API contract per block type:
//   - text:        { type, text }         — "id" optional, "name"/"input"/"tool_use_id"/"content"/"is_error" FORBIDDEN
//   - tool_use:    { type, id, name, input } — "text"/"tool_use_id"/"content"/"is_error" FORBIDDEN
//   - tool_result: { type, tool_use_id, content [, is_error] } — "id"/"name"/"input"/"text" FORBIDDEN
func TestAnthropicAPIFieldContract(t *testing.T) {
	t.Run("text block field contract", func(t *testing.T) {
		block := ContentBlock{
			Type: ContentText,
			ID:   "", // empty ID should not appear
			Text: "hello world",
		}
		b, err := json.Marshal(block)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		s := string(b)
		t.Logf("text block JSON: %s", s)

		// Required fields
		assertHas(t, s, `"type":"text"`)
		assertHas(t, s, `"text":"hello world"`)

		// Forbidden fields for text blocks
		assertNotHas(t, s, `"input"`)
		assertNotHas(t, s, `"tool_use_id"`)
		assertNotHas(t, s, `"is_error"`)
		// "id" should not appear when empty
		assertNotHas(t, s, `"id"`)
		// "name" should not appear when empty
		assertNotHas(t, s, `"name"`)
	})

	t.Run("text block with ID field contract", func(t *testing.T) {
		block := ContentBlock{
			Type: ContentText,
			ID:   "txt_123",
			Text: "hello",
		}
		b, err := json.Marshal(block)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		s := string(b)
		t.Logf("text block with ID JSON: %s", s)

		// ID should appear when non-empty
		assertHas(t, s, `"id":"txt_123"`)
		// Still no tool fields
		assertNotHas(t, s, `"input"`)
		assertNotHas(t, s, `"tool_use_id"`)
		assertNotHas(t, s, `"content"`)
	})

	t.Run("tool_use block field contract", func(t *testing.T) {
		block := ContentBlock{
			Type:  ContentToolUse,
			ID:    "toolu_abc123",
			Name:  "bash",
			Input: json.RawMessage(`{"command":"ls -la"}`),
			// These fields should NEVER appear for tool_use:
			Text:      "should not appear",
			ToolUseID: "should not appear",
			Content:   "should not appear",
			IsError:   true,
		}
		b, err := json.Marshal(block)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		s := string(b)
		t.Logf("tool_use block JSON: %s", s)

		// Required fields
		assertHas(t, s, `"type":"tool_use"`)
		assertHas(t, s, `"id":"toolu_abc123"`)
		assertHas(t, s, `"name":"bash"`)
		assertHas(t, s, `"command":"ls -la"`)

		// Forbidden fields — these would cause API errors if sent
		assertNotHas(t, s, `"text"`)        // text is for text blocks only
		assertNotHas(t, s, `"tool_use_id"`) // tool_use_id is for tool_result only
		assertNotHas(t, s, `"content"`)     // content is for tool_result only
		assertNotHas(t, s, `"is_error"`)    // is_error is for tool_result only
	})

	t.Run("tool_use block with nil input field contract", func(t *testing.T) {
		block := ContentBlock{
			Type:  ContentToolUse,
			ID:    "toolu_nil",
			Name:  "read_file",
			Input: nil, // Must default to {}
		}
		b, err := json.Marshal(block)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		s := string(b)
		t.Logf("tool_use nil input JSON: %s", s)

		assertHas(t, s, `"type":"tool_use"`)
		assertHas(t, s, `"input":{}`) // Must be {} not null
		assertHas(t, s, `"id":"toolu_nil"`)
		assertHas(t, s, `"name":"read_file"`)
	})

	t.Run("tool_result block field contract", func(t *testing.T) {
		block := ContentBlock{
			Type:      ContentToolResult,
			ToolUseID: "toolu_abc123",
			Content:   "file contents here",
			IsError:   false,
			// These fields should NEVER appear for tool_result:
			ID:   "must_not_appear_id",   // CRITICAL: causes 500 error
			Name: "must_not_appear_name",
			Input: json.RawMessage(`{}`),
			Text:  "must_not_appear_text",
		}
		b, err := json.Marshal(block)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		s := string(b)
		t.Logf("tool_result block JSON: %s", s)

		// Required fields
		assertHas(t, s, `"type":"tool_result"`)
		assertHas(t, s, `"tool_use_id":"toolu_abc123"`)
		assertHas(t, s, `"content":"file contents here"`)

		// CRITICAL: Forbidden fields — these cause 500 API errors
		assertNotHas(t, s, `"id"`)    // "'ClaudeContentBlockToolResult' object has no attribute 'id'"
		assertNotHas(t, s, `"name"`)  // name is for tool_use only
		assertNotHas(t, s, `"input"`) // input is for tool_use only
		assertNotHas(t, s, `"text"`)  // text is for text blocks only
	})

	t.Run("tool_result error block field contract", func(t *testing.T) {
		block := ContentBlock{
			Type:      ContentToolResult,
			ToolUseID: "toolu_err_456",
			Content:   "command failed with exit code 1",
			IsError:   true,
			// Forbidden fields
			ID:   "must_not_appear",
			Name: "must_not_appear",
		}
		b, err := json.Marshal(block)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		s := string(b)
		t.Logf("tool_result error block JSON: %s", s)

		assertHas(t, s, `"type":"tool_result"`)
		assertHas(t, s, `"tool_use_id":"toolu_err_456"`)
		assertHas(t, s, `"content":"command failed with exit code 1"`)
		assertHas(t, s, `"is_error":true`)

		// Forbidden
		assertNotHas(t, s, `"id"`)
		assertNotHas(t, s, `"name"`)
		assertNotHas(t, s, `"input"`)
		assertNotHas(t, s, `"text"`)
	})

	t.Run("tool_result empty content field contract", func(t *testing.T) {
		block := ContentBlock{
			Type:      ContentToolResult,
			ToolUseID: "toolu_empty",
			Content:   "", // empty but must still be present
			IsError:   false,
			ID:        "must_not_appear",
		}
		b, err := json.Marshal(block)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		s := string(b)
		t.Logf("tool_result empty content JSON: %s", s)

		// Content must be present even when empty
		assertHas(t, s, `"content":""`)
		assertHas(t, s, `"tool_use_id":"toolu_empty"`)

		// Forbidden
		assertNotHas(t, s, `"id"`)
	})

	t.Run("full conversation round-trip field contract", func(t *testing.T) {
		// Simulate a complete conversation that would be sent to the API
		msgs := []Message{
			// User asks a question
			{Role: RoleUser, Content: []ContentBlock{
				NewTextBlock("list files in current directory"),
			}},
			// Assistant responds with tool call
			{Role: RoleAssistant, Content: []ContentBlock{
				NewTextBlock("I'll list the files for you."),
				NewToolUseBlock("toolu_001", "bash", json.RawMessage(`{"command":"ls"}`)),
			}},
			// User provides tool result (per Anthropic API convention)
			{Role: RoleUser, Content: []ContentBlock{
				NewToolResultBlock("toolu_001", "file1.txt\nfile2.go\ngo.mod", false),
			}},
			// Assistant gives final answer
			{Role: RoleAssistant, Content: []ContentBlock{
				NewTextBlock("The directory contains 3 files: file1.txt, file2.go, and go.mod."),
			}},
		}

		b, err := json.Marshal(msgs)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		s := string(b)
		t.Logf("Full conversation JSON: %s", s)

		// Verify tool_use has id but tool_result does NOT have id
		// Count occurrences of "id" to ensure it only appears for tool_use
		toolUseBlock := `"type":"tool_use","id":"toolu_001","name":"bash"`
		toolResultBlock := `"type":"tool_result","tool_use_id":"toolu_001","content":"file1.txt`

		assertHas(t, s, toolUseBlock)
		assertHas(t, s, toolResultBlock)

		// The tool_result block must not have an "id" field
		// Extract just the tool_result portion and verify
		idx := strings.Index(s, `"type":"tool_result"`)
		if idx == -1 {
			t.Fatal("tool_result block not found in conversation")
		}
		// Look at the next ~200 chars
		end := idx + 200
		if end > len(s) {
			end = len(s)
		}
		toolResultSection := s[idx:end]
		assertNotHas(t, toolResultSection, `"id"`)
	})
}

// TestAsAPIMessagesFieldContract validates the full session-to-API-messages
// pipeline to ensure no invalid fields leak through from persisted sessions.
func TestAsAPIMessagesFieldContract(t *testing.T) {
	// This simulates loading a session from disk where content blocks
	// might have all fields populated, then serializing for the API.
	// The AsAPIMessages method + MarshalJSON must produce clean output.

	// Create a message with all fields set (simulating loaded session data)
	// and verify the serialized output is valid for the Anthropic API.
	t.Run("session with polluted tool_result block", func(t *testing.T) {
		// Simulate a block loaded from JSON that has all fields
		raw := `{
			"type": "tool_result",
			"id": "polluted_id",
			"name": "polluted_name",
			"input": {"command":"ls"},
			"tool_use_id": "toolu_clean",
			"content": "clean output",
			"is_error": false
		}`

		var block ContentBlock
		if err := json.Unmarshal([]byte(raw), &block); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}

		// Verify the block was parsed correctly
		if block.Type != ContentToolResult {
			t.Fatalf("Type = %v, want tool_result", block.Type)
		}
		if block.ID != "polluted_id" {
			t.Logf("Note: ID field is %q (loaded from JSON), will be stripped by MarshalJSON", block.ID)
		}

		// Now re-marshal it — MarshalJSON must strip the "id" field
		b, err := json.Marshal(block)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		s := string(b)
		t.Logf("Re-serialized tool_result: %s", s)

		// Must NOT have id, name, input
		assertNotHas(t, s, `"id"`)
		assertNotHas(t, s, `"name"`)
		assertNotHas(t, s, `"input"`)

		// Must have required fields
		assertHas(t, s, `"type":"tool_result"`)
		assertHas(t, s, `"tool_use_id":"toolu_clean"`)
		assertHas(t, s, `"content":"clean output"`)
	})

	t.Run("session with polluted tool_use block", func(t *testing.T) {
		raw := `{
			"type": "tool_use",
			"id": "toolu_123",
			"name": "bash",
			"input": {"command":"ls"},
			"tool_use_id": "polluted_tool_use_id",
			"content": "polluted_content",
			"text": "polluted_text"
		}`

		var block ContentBlock
		if err := json.Unmarshal([]byte(raw), &block); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}

		b, err := json.Marshal(block)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		s := string(b)
		t.Logf("Re-serialized tool_use: %s", s)

		// Must have required fields
		assertHas(t, s, `"type":"tool_use"`)
		assertHas(t, s, `"id":"toolu_123"`)
		assertHas(t, s, `"name":"bash"`)
		assertHas(t, s, `"command":"ls"`)

		// Must NOT have tool_result/text fields
		assertNotHas(t, s, `"tool_use_id"`)
		assertNotHas(t, s, `"content"`)
		assertNotHas(t, s, `"is_error"`)
		// Note: "text" will be omitted because it's empty string with omitempty
		// (the polluted value is handled by the switch in MarshalJSON)
	})
}

func assertHas(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected JSON to contain %q, got: %s", substr, s)
	}
}

func assertNotHas(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("expected JSON NOT to contain %q, got: %s", substr, s)
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
