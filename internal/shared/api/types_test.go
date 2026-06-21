package api

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// ContentBlock.MarshalJSON — text blocks
// ---------------------------------------------------------------------------

func TestContentBlock_MarshalJSON_TextBlock(t *testing.T) {
	t.Run("serializes type and text", func(t *testing.T) {
		b := NewTextBlock("hello world")
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if m["type"] != "text" {
			t.Errorf("type = %v, want text", m["type"])
		}
		if m["text"] != "hello world" {
			t.Errorf("text = %v, want hello world", m["text"])
		}
	})

	t.Run("always includes text field even when empty", func(t *testing.T) {
		b := NewTextBlock("")
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// The custom marshaler uses *string, so "text" must be present as null or ""
		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if _, ok := m["text"]; !ok {
			t.Error("text key missing from marshaled output")
		}
	})

	t.Run("does not include tool_use fields", func(t *testing.T) {
		b := NewTextBlock("hi")
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		for _, key := range []string{"id", "name", "input", "tool_use_id", "content", "is_error"} {
			if _, ok := m[key]; ok {
				t.Errorf("unexpected key %q in text block output", key)
			}
		}
	})

	t.Run("includes id when set", func(t *testing.T) {
		b := ContentBlock{Type: ContentText, Text: "hi", ID: "txt_123"}
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if m["id"] != "txt_123" {
			t.Errorf("id = %v, want txt_123", m["id"])
		}
	})

	t.Run("omits id when empty", func(t *testing.T) {
		b := NewTextBlock("hi")
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if _, ok := m["id"]; ok {
			t.Error("id should be omitted when empty")
		}
	})

	t.Run("exact JSON output", func(t *testing.T) {
		b := ContentBlock{Type: ContentText, Text: "hello", ID: "id1"}
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := `{"type":"text","text":"hello","id":"id1"}`
		if string(data) != expected {
			t.Errorf("got %s, want %s", string(data), expected)
		}
	})
}

// ---------------------------------------------------------------------------
// ContentBlock.MarshalJSON — tool_use blocks
// ---------------------------------------------------------------------------

func TestContentBlock_MarshalJSON_ToolUseBlock(t *testing.T) {
	t.Run("serializes all required fields", func(t *testing.T) {
		b := NewToolUseBlock("tu_1", "search", json.RawMessage(`{"q":"go"}`))
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if m["type"] != "tool_use" {
			t.Errorf("type = %v, want tool_use", m["type"])
		}
		if m["id"] != "tu_1" {
			t.Errorf("id = %v, want tu_1", m["id"])
		}
		if m["name"] != "search" {
			t.Errorf("name = %v, want search", m["name"])
		}
	})

	t.Run("always includes input field defaulting to empty object", func(t *testing.T) {
		b := NewToolUseBlock("tu_2", "run", nil)
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		input, ok := m["input"]
		if !ok {
			t.Fatal("input key missing")
		}
		inputObj, ok := input.(map[string]interface{})
		if !ok {
			t.Fatalf("input is %T, expected map", input)
		}
		if len(inputObj) != 0 {
			t.Errorf("input = %v, want empty object", inputObj)
		}
	})

	t.Run("preserves input when provided", func(t *testing.T) {
		input := json.RawMessage(`{"key":"value","num":42}`)
		b := NewToolUseBlock("tu_3", "tool", input)
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		inputObj := m["input"].(map[string]interface{})
		if inputObj["key"] != "value" {
			t.Errorf("input.key = %v, want value", inputObj["key"])
		}
		if inputObj["num"] != float64(42) {
			t.Errorf("input.num = %v, want 42", inputObj["num"])
		}
	})

	t.Run("does not include tool_result fields", func(t *testing.T) {
		b := NewToolUseBlock("tu_4", "tool", nil)
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		for _, key := range []string{"text", "tool_use_id", "content", "is_error"} {
			if _, ok := m[key]; ok {
				t.Errorf("unexpected key %q in tool_use block output", key)
			}
		}
	})

	t.Run("exact JSON output with input", func(t *testing.T) {
		b := NewToolUseBlock("id99", "calc", json.RawMessage(`{"expr":"1+1"}`))
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := `{"type":"tool_use","id":"id99","name":"calc","input":{"expr":"1+1"}}`
		if string(data) != expected {
			t.Errorf("got %s, want %s", string(data), expected)
		}
	})

	t.Run("exact JSON output with nil input", func(t *testing.T) {
		b := NewToolUseBlock("id100", "noop", nil)
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := `{"type":"tool_use","id":"id100","name":"noop","input":{}}`
		if string(data) != expected {
			t.Errorf("got %s, want %s", string(data), expected)
		}
	})
}

// ---------------------------------------------------------------------------
// ContentBlock.MarshalJSON — tool_result blocks
// ---------------------------------------------------------------------------

func TestContentBlock_MarshalJSON_ToolResultBlock(t *testing.T) {
	t.Run("serializes all required fields", func(t *testing.T) {
		b := NewToolResultBlock("tu_1", "result text", false)
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if m["type"] != "tool_result" {
			t.Errorf("type = %v, want tool_result", m["type"])
		}
		if m["tool_use_id"] != "tu_1" {
			t.Errorf("tool_use_id = %v, want tu_1", m["tool_use_id"])
		}
		if m["content"] != "result text" {
			t.Errorf("content = %v, want result text", m["content"])
		}
	})

	t.Run("omits is_error when false", func(t *testing.T) {
		b := NewToolResultBlock("tu_2", "ok", false)
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if strings.Contains(string(data), "is_error") {
			t.Error("is_error should be omitted when false")
		}
	})

	t.Run("includes is_error when true", func(t *testing.T) {
		b := NewToolResultBlock("tu_3", "failed", true)
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if m["is_error"] != true {
			t.Errorf("is_error = %v, want true", m["is_error"])
		}
	})

	t.Run("never includes id name or input", func(t *testing.T) {
		// Even if someone sets those fields on a tool_result block, they
		// should not appear in JSON output.
		b := ContentBlock{
			Type:      ContentToolResult,
			ToolUseID: "tu_4",
			Content:   "c",
			ID:        "should-not-appear",
			Name:      "should-not-appear",
			Input:     json.RawMessage(`{"no":"pe"}`),
		}
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		for _, key := range []string{"id", "name", "input", "text"} {
			if _, ok := m[key]; ok {
				t.Errorf("unexpected key %q in tool_result block output", key)
			}
		}
	})

	t.Run("exact JSON output success", func(t *testing.T) {
		b := NewToolResultBlock("tu_5", "all good", false)
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := `{"type":"tool_result","tool_use_id":"tu_5","content":"all good"}`
		if string(data) != expected {
			t.Errorf("got %s, want %s", string(data), expected)
		}
	})

	t.Run("exact JSON output error", func(t *testing.T) {
		b := NewToolResultBlock("tu_6", "oops", true)
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := `{"type":"tool_result","tool_use_id":"tu_6","content":"oops","is_error":true}`
		if string(data) != expected {
			t.Errorf("got %s, want %s", string(data), expected)
		}
	})
}

// ---------------------------------------------------------------------------
// ContentBlock.MarshalJSON — default / unknown type
// ---------------------------------------------------------------------------

func TestContentBlock_MarshalJSON_DefaultType(t *testing.T) {
	t.Run("unknown type serializes as generic block", func(t *testing.T) {
		b := ContentBlock{Type: ContentBlockType("custom_type")}
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := `{"type":"custom_type"}`
		if string(data) != expected {
			t.Errorf("got %s, want %s", string(data), expected)
		}
	})

	t.Run("zero type serializes as empty type string", func(t *testing.T) {
		b := ContentBlock{}
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := `{"type":""}`
		if string(data) != expected {
			t.Errorf("got %s, want %s", string(data), expected)
		}
	})
}

// ---------------------------------------------------------------------------
// NewTextBlock
// ---------------------------------------------------------------------------

func TestNewTextBlock(t *testing.T) {
	t.Run("sets type to text", func(t *testing.T) {
		b := NewTextBlock("hello")
		if b.Type != ContentText {
			t.Errorf("Type = %v, want %v", b.Type, ContentText)
		}
	})

	t.Run("sets text content", func(t *testing.T) {
		b := NewTextBlock("expected text")
		if b.Text != "expected text" {
			t.Errorf("Text = %q, want %q", b.Text, "expected text")
		}
	})

	t.Run("empty string is valid", func(t *testing.T) {
		b := NewTextBlock("")
		if b.Text != "" {
			t.Errorf("Text = %q, want empty", b.Text)
		}
		if b.Type != ContentText {
			t.Errorf("Type = %v, want %v", b.Type, ContentText)
		}
	})

	t.Run("no other fields are set", func(t *testing.T) {
		b := NewTextBlock("hi")
		if b.ID != "" {
			t.Errorf("ID = %q, want empty", b.ID)
		}
		if b.Name != "" {
			t.Errorf("Name = %q, want empty", b.Name)
		}
		if b.Input != nil {
			t.Errorf("Input = %v, want nil", b.Input)
		}
		if b.ToolUseID != "" {
			t.Errorf("ToolUseID = %q, want empty", b.ToolUseID)
		}
		if b.Content != "" {
			t.Errorf("Content = %q, want empty", b.Content)
		}
		if b.IsError {
			t.Error("IsError = true, want false")
		}
	})
}

// ---------------------------------------------------------------------------
// NewToolUseBlock
// ---------------------------------------------------------------------------

func TestNewToolUseBlock(t *testing.T) {
	t.Run("sets all fields", func(t *testing.T) {
		input := json.RawMessage(`{"a":1}`)
		b := NewToolUseBlock("id1", "tool1", input)
		if b.Type != ContentToolUse {
			t.Errorf("Type = %v, want %v", b.Type, ContentToolUse)
		}
		if b.ID != "id1" {
			t.Errorf("ID = %q, want %q", b.ID, "id1")
		}
		if b.Name != "tool1" {
			t.Errorf("Name = %q, want %q", b.Name, "tool1")
		}
		if string(b.Input) != `{"a":1}` {
			t.Errorf("Input = %s, want %s", string(b.Input), `{"a":1}`)
		}
	})

	t.Run("nil input defaults to empty object", func(t *testing.T) {
		b := NewToolUseBlock("id2", "tool2", nil)
		if string(b.Input) != `{}` {
			t.Errorf("Input = %s, want {}", string(b.Input))
		}
	})

	t.Run("empty input is preserved", func(t *testing.T) {
		input := json.RawMessage(`{}`)
		b := NewToolUseBlock("id3", "tool3", input)
		if string(b.Input) != `{}` {
			t.Errorf("Input = %s, want {}", string(b.Input))
		}
	})

	t.Run("complex input is preserved", func(t *testing.T) {
		input := json.RawMessage(`{"nested":{"deep":true},"arr":[1,2,3]}`)
		b := NewToolUseBlock("id4", "tool4", input)
		if string(b.Input) != `{"nested":{"deep":true},"arr":[1,2,3]}` {
			t.Errorf("Input = %s, want original", string(b.Input))
		}
	})

	t.Run("no unrelated fields are set", func(t *testing.T) {
		b := NewToolUseBlock("id5", "tool5", nil)
		if b.Text != "" {
			t.Errorf("Text = %q, want empty", b.Text)
		}
		if b.ToolUseID != "" {
			t.Errorf("ToolUseID = %q, want empty", b.ToolUseID)
		}
		if b.Content != "" {
			t.Errorf("Content = %q, want empty", b.Content)
		}
		if b.IsError {
			t.Error("IsError = true, want false")
		}
	})
}

// ---------------------------------------------------------------------------
// NewToolResultBlock
// ---------------------------------------------------------------------------

func TestNewToolResultBlock(t *testing.T) {
	t.Run("sets all fields for success result", func(t *testing.T) {
		b := NewToolResultBlock("tu_abc", "done", false)
		if b.Type != ContentToolResult {
			t.Errorf("Type = %v, want %v", b.Type, ContentToolResult)
		}
		if b.ToolUseID != "tu_abc" {
			t.Errorf("ToolUseID = %q, want %q", b.ToolUseID, "tu_abc")
		}
		if b.Content != "done" {
			t.Errorf("Content = %q, want %q", b.Content, "done")
		}
		if b.IsError {
			t.Error("IsError = true, want false")
		}
	})

	t.Run("sets all fields for error result", func(t *testing.T) {
		b := NewToolResultBlock("tu_err", "something broke", true)
		if b.Type != ContentToolResult {
			t.Errorf("Type = %v, want %v", b.Type, ContentToolResult)
		}
		if b.ToolUseID != "tu_err" {
			t.Errorf("ToolUseID = %q, want %q", b.ToolUseID, "tu_err")
		}
		if b.Content != "something broke" {
			t.Errorf("Content = %q, want %q", b.Content, "something broke")
		}
		if !b.IsError {
			t.Error("IsError = false, want true")
		}
	})

	t.Run("empty content is valid", func(t *testing.T) {
		b := NewToolResultBlock("tu_x", "", false)
		if b.Content != "" {
			t.Errorf("Content = %q, want empty", b.Content)
		}
	})

	t.Run("no unrelated fields are set", func(t *testing.T) {
		b := NewToolResultBlock("tu_y", "ok", false)
		if b.Text != "" {
			t.Errorf("Text = %q, want empty", b.Text)
		}
		if b.ID != "" {
			t.Errorf("ID = %q, want empty", b.ID)
		}
		if b.Name != "" {
			t.Errorf("Name = %q, want empty", b.Name)
		}
		if b.Input != nil {
			t.Errorf("Input = %v, want nil", b.Input)
		}
	})
}

// ---------------------------------------------------------------------------
// Zero-value / default behavior
// ---------------------------------------------------------------------------

func TestContentBlock_ZeroValue(t *testing.T) {
	var b ContentBlock
	if b.Type != "" {
		t.Errorf("Type = %q, want empty", b.Type)
	}
	if b.Text != "" {
		t.Errorf("Text = %q, want empty", b.Text)
	}
	if b.ID != "" {
		t.Errorf("ID = %q, want empty", b.ID)
	}
	if b.Name != "" {
		t.Errorf("Name = %q, want empty", b.Name)
	}
	if b.Input != nil {
		t.Errorf("Input = %v, want nil", b.Input)
	}
	if b.ToolUseID != "" {
		t.Errorf("ToolUseID = %q, want empty", b.ToolUseID)
	}
	if b.Content != "" {
		t.Errorf("Content = %q, want empty", b.Content)
	}
	if b.IsError {
		t.Error("IsError = true, want false")
	}
}

func TestMessage_ZeroValue(t *testing.T) {
	var m Message
	if m.Role != "" {
		t.Errorf("Role = %q, want empty", m.Role)
	}
	if m.Content != nil {
		t.Errorf("Content = %v, want nil", m.Content)
	}
}

func TestUsage_ZeroValue(t *testing.T) {
	var u Usage
	if u.InputTokens != 0 {
		t.Errorf("InputTokens = %d, want 0", u.InputTokens)
	}
	if u.OutputTokens != 0 {
		t.Errorf("OutputTokens = %d, want 0", u.OutputTokens)
	}
	if u.CacheCreationInputTokens != 0 {
		t.Errorf("CacheCreationInputTokens = %d, want 0", u.CacheCreationInputTokens)
	}
	if u.CacheReadInputTokens != 0 {
		t.Errorf("CacheReadInputTokens = %d, want 0", u.CacheReadInputTokens)
	}
}

func TestRequest_ZeroValue(t *testing.T) {
	var r Request
	if r.Model != "" {
		t.Errorf("Model = %q, want empty", r.Model)
	}
	if r.Messages != nil {
		t.Errorf("Messages = %v, want nil", r.Messages)
	}
	if r.Tools != nil {
		t.Errorf("Tools = %v, want nil", r.Tools)
	}
	if r.MaxTokens != 0 {
		t.Errorf("MaxTokens = %d, want 0", r.MaxTokens)
	}
	if r.Temperature != 0 {
		t.Errorf("Temperature = %v, want 0", r.Temperature)
	}
	if r.Stream {
		t.Error("Stream = true, want false")
	}
	if r.System != "" {
		t.Errorf("System = %q, want empty", r.System)
	}
	if r.ToolChoice != "" {
		t.Errorf("ToolChoice = %q, want empty", r.ToolChoice)
	}
}

func TestResponse_ZeroValue(t *testing.T) {
	var r Response
	if r.ID != "" {
		t.Errorf("ID = %q, want empty", r.ID)
	}
	if r.Content != nil {
		t.Errorf("Content = %v, want nil", r.Content)
	}
	if r.StopReason != "" {
		t.Errorf("StopReason = %q, want empty", r.StopReason)
	}
	// Usage is a struct so its zero value has all-zero fields
	if r.Model != "" {
		t.Errorf("Model = %q, want empty", r.Model)
	}
	if r.RequestID != "" {
		t.Errorf("RequestID = %q, want empty", r.RequestID)
	}
}

// ---------------------------------------------------------------------------
// Edge cases — special characters, long content, round-trip
// ---------------------------------------------------------------------------

func TestContentBlock_SpecialCharacters(t *testing.T) {
	tests := []struct {
		name  string
		block ContentBlock
	}{
		{
			name:  "text with quotes and newlines",
			block: NewTextBlock(`line1\nline2 "quoted" & <tag>`),
		},
		{
			name:  "text with unicode",
			block: NewTextBlock("Hello 世界 🌍 Ñoño"),
		},
		{
			name:  "text with backslashes",
			block: NewTextBlock(`C:\Users\test\file.txt`),
		},
		{
			name:  "tool result with special chars",
			block: NewToolResultBlock("tu_1", `{"key":"val"}`, false),
		},
		{
			name:  "tool use with special chars in name",
			block: NewToolUseBlock("id1", "my-tool_v2", json.RawMessage(`{"q":"test\"quote"}`)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.block)
			if err != nil {
				t.Fatalf("MarshalJSON error: %v", err)
			}

			// Verify it's valid JSON by round-tripping through unmarshal
			if !json.Valid(data) {
				t.Fatalf("invalid JSON: %s", string(data))
			}
		})
	}
}

func TestContentBlock_VeryLongContent(t *testing.T) {
	t.Run("long text block", func(t *testing.T) {
		longText := strings.Repeat("a", 1_000_000)
		b := NewTextBlock(longText)
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("MarshalJSON error: %v", err)
		}
		if len(data) < 1_000_000 {
			t.Errorf("output too short: %d", len(data))
		}
	})

	t.Run("long tool result content", func(t *testing.T) {
		longContent := strings.Repeat("x", 100_000)
		b := NewToolResultBlock("tu_long", longContent, false)
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("MarshalJSON error: %v", err)
		}
		if len(data) < 100_000 {
			t.Errorf("output too short: %d", len(data))
		}
	})

	t.Run("large tool input", func(t *testing.T) {
		// Build a large JSON object
		parts := make([]string, 1000)
		for i := 0; i < 1000; i++ {
			parts[i] = `"k":"v"`
		}
		largeInput := json.RawMessage(`{` + strings.Join(parts, ",") + `}`)
		b := NewToolUseBlock("id_big", "big_tool", largeInput)
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("MarshalJSON error: %v", err)
		}
		if len(data) < 5000 {
			t.Errorf("output seems too short: %d", len(data))
		}
	})
}

func TestContentBlock_EmptyNames(t *testing.T) {
	t.Run("tool_use with empty id and name", func(t *testing.T) {
		b := NewToolUseBlock("", "", nil)
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("MarshalJSON error: %v", err)
		}

		expected := `{"type":"tool_use","id":"","name":"","input":{}}`
		if string(data) != expected {
			t.Errorf("got %s, want %s", string(data), expected)
		}
	})

	t.Run("tool_result with empty tool_use_id and content", func(t *testing.T) {
		b := NewToolResultBlock("", "", false)
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("MarshalJSON error: %v", err)
		}

		expected := `{"type":"tool_result","tool_use_id":"","content":""}`
		if string(data) != expected {
			t.Errorf("got %s, want %s", string(data), expected)
		}
	})
}

// ---------------------------------------------------------------------------
// Full JSON round-trip (marshal then unmarshal) for struct types
// ---------------------------------------------------------------------------

func TestMessage_RoundTrip(t *testing.T) {
	original := Message{
		Role: RoleUser,
		Content: []ContentBlock{
			NewTextBlock("What is 2+2?"),
			NewToolResultBlock("tu_1", "4", false),
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Role != RoleUser {
		t.Errorf("Role = %q, want %q", decoded.Role, RoleUser)
	}
	if len(decoded.Content) != 2 {
		t.Fatalf("len(Content) = %d, want 2", len(decoded.Content))
	}
	if decoded.Content[0].Type != ContentText {
		t.Errorf("Content[0].Type = %v, want %v", decoded.Content[0].Type, ContentText)
	}
	if decoded.Content[0].Text != "What is 2+2?" {
		t.Errorf("Content[0].Text = %q, want %q", decoded.Content[0].Text, "What is 2+2?")
	}
	if decoded.Content[1].Type != ContentToolResult {
		t.Errorf("Content[1].Type = %v, want %v", decoded.Content[1].Type, ContentToolResult)
	}
	if decoded.Content[1].ToolUseID != "tu_1" {
		t.Errorf("Content[1].ToolUseID = %q, want tu_1", decoded.Content[1].ToolUseID)
	}
	if decoded.Content[1].Content != "4" {
		t.Errorf("Content[1].Content = %q, want 4", decoded.Content[1].Content)
	}
}

func TestRequest_RoundTrip(t *testing.T) {
	original := Request{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 4096,
		Messages: []Message{
			{Role: RoleUser, Content: []ContentBlock{NewTextBlock("hello")}},
		},
		Tools: []ToolDefinition{
			{
				Name:        "search",
				Description: "Search the web",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`),
			},
		},
		Temperature: 0.7,
		Stream:      true,
		System:      "You are helpful.",
		ToolChoice:  ToolChoiceAuto,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Request
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Model != original.Model {
		t.Errorf("Model = %q, want %q", decoded.Model, original.Model)
	}
	if decoded.MaxTokens != original.MaxTokens {
		t.Errorf("MaxTokens = %d, want %d", decoded.MaxTokens, original.MaxTokens)
	}
	if decoded.Stream != original.Stream {
		t.Errorf("Stream = %v, want %v", decoded.Stream, original.Stream)
	}
	if decoded.Temperature != original.Temperature {
		t.Errorf("Temperature = %v, want %v", decoded.Temperature, original.Temperature)
	}
	if decoded.System != original.System {
		t.Errorf("System = %q, want %q", decoded.System, original.System)
	}
	if decoded.ToolChoice != original.ToolChoice {
		t.Errorf("ToolChoice = %q, want %q", decoded.ToolChoice, original.ToolChoice)
	}
	if len(decoded.Messages) != 1 {
		t.Fatalf("len(Messages) = %d, want 1", len(decoded.Messages))
	}
	if decoded.Messages[0].Role != RoleUser {
		t.Errorf("Messages[0].Role = %q, want user", decoded.Messages[0].Role)
	}
	if len(decoded.Tools) != 1 {
		t.Fatalf("len(Tools) = %d, want 1", len(decoded.Tools))
	}
	if decoded.Tools[0].Name != "search" {
		t.Errorf("Tools[0].Name = %q, want search", decoded.Tools[0].Name)
	}
}

func TestResponse_RoundTrip(t *testing.T) {
	original := Response{
		ID:         "resp_123",
		StopReason: StopEndTurn,
		Usage:      Usage{InputTokens: 100, OutputTokens: 50},
		Content:    []ContentBlock{NewTextBlock("Hi there")},
		Model:      "claude-3-5-sonnet-20241022",
		RequestID:  "internal-request-id", // should NOT appear in JSON
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	// Verify RequestID is NOT in the JSON output (json:"-")
	if strings.Contains(string(data), "internal-request-id") {
		t.Error("RequestID should not appear in JSON output")
	}

	var decoded Response
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.ID != "resp_123" {
		t.Errorf("ID = %q, want resp_123", decoded.ID)
	}
	if decoded.StopReason != StopEndTurn {
		t.Errorf("StopReason = %q, want end_turn", decoded.StopReason)
	}
	if decoded.Usage.InputTokens != 100 {
		t.Errorf("Usage.InputTokens = %d, want 100", decoded.Usage.InputTokens)
	}
	if decoded.Usage.OutputTokens != 50 {
		t.Errorf("Usage.OutputTokens = %d, want 50", decoded.Usage.OutputTokens)
	}
	if decoded.Model != "claude-3-5-sonnet-20241022" {
		t.Errorf("Model = %q, want claude-3-5-sonnet-20241022", decoded.Model)
	}
}

func TestUsage_OmitemptyFields(t *testing.T) {
	t.Run("cache fields omitted when zero", func(t *testing.T) {
		u := Usage{InputTokens: 10, OutputTokens: 5}
		data, err := json.Marshal(u)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}

		if strings.Contains(string(data), "cache_creation") {
			t.Error("cache_creation_input_tokens should be omitted when 0")
		}
		if strings.Contains(string(data), "cache_read") {
			t.Error("cache_read_input_tokens should be omitted when 0")
		}
	})

	t.Run("cache fields present when non-zero", func(t *testing.T) {
		u := Usage{
			InputTokens:              10,
			OutputTokens:             5,
			CacheCreationInputTokens: 3,
			CacheReadInputTokens:     7,
		}
		data, err := json.Marshal(u)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}

		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}

		if m["cache_creation_input_tokens"] != float64(3) {
			t.Errorf("cache_creation_input_tokens = %v, want 3", m["cache_creation_input_tokens"])
		}
		if m["cache_read_input_tokens"] != float64(7) {
			t.Errorf("cache_read_input_tokens = %v, want 7", m["cache_read_input_tokens"])
		}
	})
}

func TestRequest_OmitemptyFields(t *testing.T) {
	t.Run("tools omitted when nil", func(t *testing.T) {
		r := Request{Model: "m", MaxTokens: 100, Stream: false}
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		if strings.Contains(string(data), "tools") {
			t.Error("tools should be omitted when nil")
		}
	})

	t.Run("tools present when non-empty", func(t *testing.T) {
		r := Request{
			Model:     "m",
			MaxTokens: 100,
			Tools:     []ToolDefinition{{Name: "t", Description: "d", InputSchema: json.RawMessage(`{}`)}},
		}
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		if !strings.Contains(string(data), `"tools"`) {
			t.Error("tools should be present when non-empty")
		}
	})

	t.Run("temperature omitted when zero", func(t *testing.T) {
		r := Request{Model: "m", MaxTokens: 100}
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		if strings.Contains(string(data), "temperature") {
			t.Error("temperature should be omitted when 0")
		}
	})

	t.Run("system omitted when empty", func(t *testing.T) {
		r := Request{Model: "m", MaxTokens: 100}
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		if strings.Contains(string(data), "system") {
			t.Error("system should be omitted when empty")
		}
	})

	t.Run("tool_choice omitted when empty", func(t *testing.T) {
		r := Request{Model: "m", MaxTokens: 100}
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		if strings.Contains(string(data), "tool_choice") {
			t.Error("tool_choice should be omitted when empty")
		}
	})
}

func TestResponse_RequestIDNotSerialized(t *testing.T) {
	r := Response{
		ID:        "r1",
		RequestID: "secret-internal-id",
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if _, ok := m["RequestID"]; ok {
		t.Error("RequestID should not appear in JSON")
	}
	if _, ok := m["request_id"]; ok {
		t.Error("request_id should not appear in JSON")
	}
}

// ---------------------------------------------------------------------------
// Constants correctness
// ---------------------------------------------------------------------------

func TestConstants(t *testing.T) {
	tests := []struct {
		name  string
		got   string
		want  string
	}{
		{"RoleUser", string(RoleUser), "user"},
		{"RoleAssistant", string(RoleAssistant), "assistant"},
		{"StopEndTurn", string(StopEndTurn), "end_turn"},
		{"StopToolUse", string(StopToolUse), "tool_use"},
		{"StopMaxTokens", string(StopMaxTokens), "max_tokens"},
		{"StopSequence", string(StopSequence), "stop_sequence"},
		{"ContentText", string(ContentText), "text"},
		{"ContentToolUse", string(ContentToolUse), "tool_use"},
		{"ContentToolResult", string(ContentToolResult), "tool_result"},
		{"ToolChoiceAuto", string(ToolChoiceAuto), "auto"},
		{"ToolChoiceAny", string(ToolChoiceAny), "any"},
		{"ToolChoiceNone", string(ToolChoiceNone), "none"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ToolDefinition
// ---------------------------------------------------------------------------

func TestToolDefinition_RoundTrip(t *testing.T) {
	original := ToolDefinition{
		Name:        "read_file",
		Description: "Read a file from disk",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded ToolDefinition
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Name != original.Name {
		t.Errorf("Name = %q, want %q", decoded.Name, original.Name)
	}
	if decoded.Description != original.Description {
		t.Errorf("Description = %q, want %q", decoded.Description, original.Description)
	}
	if string(decoded.InputSchema) != string(original.InputSchema) {
		t.Errorf("InputSchema = %s, want %s", string(decoded.InputSchema), string(original.InputSchema))
	}
}

// ---------------------------------------------------------------------------
// Full conversation flow (integration-style)
// ---------------------------------------------------------------------------

func TestFullConversationFlow(t *testing.T) {
	// Simulate: user message → assistant tool_use → tool_result → assistant text

	userMsg := Message{
		Role:    RoleUser,
		Content: []ContentBlock{NewTextBlock("What's the weather in NYC?")},
	}

	assistantMsg := Message{
		Role: RoleAssistant,
		Content: []ContentBlock{
			NewToolUseBlock("tu_weather", "get_weather", json.RawMessage(`{"city":"NYC"}`)),
		},
	}

	toolResultMsg := Message{
		Role:    RoleUser,
		Content: []ContentBlock{NewToolResultBlock("tu_weather", "72°F, sunny", false)},
	}

	finalMsg := Message{
		Role:    RoleAssistant,
		Content: []ContentBlock{NewTextBlock("The weather in NYC is 72°F and sunny!")},
	}

	messages := []Message{userMsg, assistantMsg, toolResultMsg, finalMsg}

	data, err := json.Marshal(messages)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded []Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if len(decoded) != 4 {
		t.Fatalf("len = %d, want 4", len(decoded))
	}

	// Verify user message
	if decoded[0].Role != RoleUser {
		t.Errorf("msg[0] Role = %q, want user", decoded[0].Role)
	}
	if decoded[0].Content[0].Text != "What's the weather in NYC?" {
		t.Errorf("msg[0] Text = %q", decoded[0].Content[0].Text)
	}

	// Verify assistant tool_use
	if decoded[1].Role != RoleAssistant {
		t.Errorf("msg[1] Role = %q, want assistant", decoded[1].Role)
	}
	if decoded[1].Content[0].Type != ContentToolUse {
		t.Errorf("msg[1] Type = %v, want tool_use", decoded[1].Content[0].Type)
	}
	if decoded[1].Content[0].ID != "tu_weather" {
		t.Errorf("msg[1] ID = %q, want tu_weather", decoded[1].Content[0].ID)
	}
	if decoded[1].Content[0].Name != "get_weather" {
		t.Errorf("msg[1] Name = %q, want get_weather", decoded[1].Content[0].Name)
	}

	// Verify tool result
	if decoded[2].Content[0].Type != ContentToolResult {
		t.Errorf("msg[2] Type = %v, want tool_result", decoded[2].Content[0].Type)
	}
	if decoded[2].Content[0].ToolUseID != "tu_weather" {
		t.Errorf("msg[2] ToolUseID = %q, want tu_weather", decoded[2].Content[0].ToolUseID)
	}
	if decoded[2].Content[0].Content != "72°F, sunny" {
		t.Errorf("msg[2] Content = %q, want 72°F, sunny", decoded[2].Content[0].Content)
	}
	if decoded[2].Content[0].IsError {
		t.Error("msg[2] IsError = true, want false")
	}

	// Verify final assistant message
	if decoded[3].Role != RoleAssistant {
		t.Errorf("msg[3] Role = %q, want assistant", decoded[3].Role)
	}
	if decoded[3].Content[0].Text != "The weather in NYC is 72°F and sunny!" {
		t.Errorf("msg[3] Text = %q", decoded[3].Content[0].Text)
	}
}
