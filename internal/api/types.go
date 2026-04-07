package api

import "encoding/json"

// MessageRole represents the role of a message sender.
type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
)

// StopReason indicates why the model stopped generating.
type StopReason string

const (
	StopEndTurn      StopReason = "end_turn"
	StopToolUse      StopReason = "tool_use"
	StopMaxTokens    StopReason = "max_tokens"
	StopSequence     StopReason = "stop_sequence"
)

// ContentBlockType enumerates the kinds of content in a message.
type ContentBlockType string

const (
	ContentText      ContentBlockType = "text"
	ContentToolUse   ContentBlockType = "tool_use"
	ContentToolResult ContentBlockType = "tool_result"
)

// ContentBlock is a sum-type representing text, tool use, or tool result.
type ContentBlock struct {
	Type      ContentBlockType  `json:"type"`
	Text      string            `json:"text,omitempty"`
	ID        string            `json:"id,omitempty"`
	Name      string            `json:"name,omitempty"`
	Input     json.RawMessage   `json:"input,omitempty"`
	ToolUseID string            `json:"tool_use_id,omitempty"`
	Content   string            `json:"content,omitempty"`
	IsError   bool              `json:"is_error,omitempty"`
}

// MarshalJSON ensures that each content block type serializes only the fields
// that the Anthropic Messages API expects for that type.
//
// Anthropic API field contract:
//   - text:        { type, text [, id] }
//   - tool_use:    { type, id, name, input }
//   - tool_result: { type, tool_use_id, content [, is_error] }
//
// Cross-type field leakage (e.g., "id" on a tool_result block) causes
// server-side 500 errors like:
//   "'ClaudeContentBlockToolResult' object has no attribute 'id'"
//
// Additionally, required fields must always be present even when empty:
//   - text blocks always include "text"
//   - tool_use blocks always include "input" (defaults to {})
//   - tool_result blocks always include "content"
func (b ContentBlock) MarshalJSON() ([]byte, error) {
	switch b.Type {
	case ContentText:
		// text: { type, text [, id] }
		type textBlock struct {
			Type string  `json:"type"`
			Text *string `json:"text"`
			ID   string  `json:"id,omitempty"`
		}
		return json.Marshal(textBlock{
			Type: "text",
			Text: &b.Text,
			ID:   b.ID,
		})

	case ContentToolUse:
		// tool_use: { type, id, name, input }
		input := b.Input
		if input == nil {
			input = json.RawMessage(`{}`)
		}
		type toolUseBlock struct {
			Type  string          `json:"type"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		}
		return json.Marshal(toolUseBlock{
			Type:  "tool_use",
			ID:    b.ID,
			Name:  b.Name,
			Input: input,
		})

	case ContentToolResult:
		// tool_result: { type, tool_use_id, content [, is_error] }
		// CRITICAL: Must NOT include "id", "name", or "input" fields.
		type toolResultBlock struct {
			Type      string `json:"type"`
			ToolUseID string `json:"tool_use_id"`
			Content   string `json:"content"`
			IsError   bool   `json:"is_error,omitempty"`
		}
		return json.Marshal(toolResultBlock{
			Type:      "tool_result",
			ToolUseID: b.ToolUseID,
			Content:   b.Content,
			IsError:   b.IsError,
		})

	default:
		// Unknown block type — use generic serialization
		type genericBlock struct {
			Type ContentBlockType `json:"type"`
		}
		return json.Marshal(genericBlock{Type: b.Type})
	}
}

// NewTextBlock creates a text content block.
func NewTextBlock(text string) ContentBlock {
	return ContentBlock{Type: ContentText, Text: text}
}

// NewToolUseBlock creates a tool_use content block.
// If input is nil, it defaults to an empty JSON object {} to avoid
// sending null to the API, which causes "expected str instance, NoneType found" errors.
func NewToolUseBlock(id, name string, input json.RawMessage) ContentBlock {
	if input == nil {
		input = json.RawMessage(`{}`)
	}
	return ContentBlock{Type: ContentToolUse, ID: id, Name: name, Input: input}
}

// NewToolResultBlock creates a tool_result content block.
func NewToolResultBlock(toolUseID, content string, isError bool) ContentBlock {
	return ContentBlock{
		Type:      ContentToolResult,
		ToolUseID: toolUseID,
		Content:   content,
		IsError:   isError,
	}
}

// Message represents a single message in the conversation.
type Message struct {
	Role    MessageRole    `json:"role"`
	Content []ContentBlock `json:"content"`
}

// ToolDefinition describes a tool available to the model.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// ToolChoice specifies how the model should choose tools.
type ToolChoice string

const (
	ToolChoiceAuto ToolChoice = "auto"
	ToolChoiceAny  ToolChoice = "any"
	ToolChoiceNone ToolChoice = "none"
)

// Usage tracks token consumption.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// Request is the API request payload.
type Request struct {
	Model      string           `json:"model"`
	Messages   []Message        `json:"messages"`
	Tools      []ToolDefinition `json:"tools,omitempty"`
	MaxTokens  int              `json:"max_tokens"`
	Temperature float64         `json:"temperature,omitempty"`
	Stream     bool             `json:"stream"`
	System     string           `json:"system,omitempty"`
	ToolChoice ToolChoice       `json:"tool_choice,omitempty"`
}

// Response is the API response payload.
type Response struct {
	ID         string        `json:"id"`
	Content    []ContentBlock `json:"content"`
	StopReason StopReason    `json:"stop_reason"`
	Usage      Usage         `json:"usage"`
	Model      string        `json:"model,omitempty"`
	RequestID  string        `json:"-"` // populated from x-request-id header
}
