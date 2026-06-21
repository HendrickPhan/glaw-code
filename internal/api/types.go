// Package api provides LLM provider client implementations.
//
// Shared protocol types (ContentBlock, Message, Request, Response, etc.)
// are defined in internal/shared/api and re-exported here for backward
// compatibility. New code should import shared/api directly.
package api

import (
	"encoding/json"

	sharedapi "github.com/hieu-glaw/glaw-code/internal/shared/api"
)

// Re-export shared types for backward compatibility.
type (
	MessageRole           = sharedapi.MessageRole
	StopReason            = sharedapi.StopReason
	ContentBlockType      = sharedapi.ContentBlockType
	ContentBlock          = sharedapi.ContentBlock
	Message               = sharedapi.Message
	ToolDefinition        = sharedapi.ToolDefinition
	ToolChoice            = sharedapi.ToolChoice
	Usage                 = sharedapi.Usage
	Request               = sharedapi.Request
	Response              = sharedapi.Response
)

// Re-export constants.
const (
	RoleUser      = sharedapi.RoleUser
	RoleAssistant = sharedapi.RoleAssistant

	StopEndTurn   = sharedapi.StopEndTurn
	StopToolUse   = sharedapi.StopToolUse
	StopMaxTokens = sharedapi.StopMaxTokens
	StopSequence  = sharedapi.StopSequence

	ContentText       = sharedapi.ContentText
	ContentToolUse    = sharedapi.ContentToolUse
	ContentToolResult = sharedapi.ContentToolResult

	ToolChoiceAuto = sharedapi.ToolChoiceAuto
	ToolChoiceAny  = sharedapi.ToolChoiceAny
	ToolChoiceNone = sharedapi.ToolChoiceNone
)

// NewTextBlock creates a text content block.
func NewTextBlock(text string) ContentBlock {
	return sharedapi.NewTextBlock(text)
}

// NewToolUseBlock creates a tool_use content block.
func NewToolUseBlock(id, name string, input json.RawMessage) ContentBlock {
	return sharedapi.NewToolUseBlock(id, name, input)
}

// NewToolResultBlock creates a tool_result content block.
func NewToolResultBlock(toolUseID, content string, isError bool) ContentBlock {
	return sharedapi.NewToolResultBlock(toolUseID, content, isError)
}
