package entity

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/hieu-glaw/glaw-code/internal/shared/api"
)

// ConversationMessage represents a single message in a conversation.
type ConversationMessage struct {
	Role   string             `json:"role"`
	Blocks []api.ContentBlock `json:"blocks"`
	Usage  *api.Usage         `json:"usage,omitempty"`
}

// Session represents a conversation session.
type Session struct {
	Version  int                  `json:"version"`
	Messages []ConversationMessage `json:"messages"`
	ID       string                `json:"id"`
	mu       sync.RWMutex
}

// NewSession creates a new empty session.
func NewSession() *Session {
	return &Session{
		Version:  1,
		Messages: []ConversationMessage{},
		ID:       fmt.Sprintf("sess_%d", time.Now().UnixMilli()),
	}
}

// AddUserMessage appends a user message with content blocks.
func (s *Session) AddUserMessage(blocks []api.ContentBlock) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, ConversationMessage{
		Role:   string(api.RoleUser),
		Blocks: blocks,
	})
}

// AddUserMessageFromText appends a user message from a text string.
func (s *Session) AddUserMessageFromText(text string) {
	s.AddUserMessage([]api.ContentBlock{api.NewTextBlock(text)})
}

// AddUserMessagesFromText appends a user message from a text string (alias).
func (s *Session) AddUserMessagesFromText(text string) {
	s.AddUserMessageFromText(text)
}

// AddAssistantMessage appends an assistant message.
func (s *Session) AddAssistantMessage(blocks []api.ContentBlock, usage *api.Usage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, ConversationMessage{
		Role:   string(api.RoleAssistant),
		Blocks: blocks,
		Usage:  usage,
	})
}

// AddToolResult adds a tool result as a new user message.
func (s *Session) AddToolResult(toolUseID, content string, isError bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, ConversationMessage{
		Role:   string(api.RoleUser),
		Blocks: []api.ContentBlock{api.NewToolResultBlock(toolUseID, content, isError)},
	})
}

// MessageCount returns the number of messages.
func (s *Session) MessageCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.Messages)
}

// AsAPIMessages converts session messages to API format.
func (s *Session) AsAPIMessages() []api.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var msgs []api.Message
	for _, m := range s.Messages {
		if len(m.Blocks) == 0 {
			continue
		}

		cleanBlocks := make([]api.ContentBlock, 0, len(m.Blocks))
		for _, b := range m.Blocks {
			switch b.Type {
			case api.ContentToolUse:
				if b.Input == nil {
					b.Input = json.RawMessage(`{}`)
				}
			case api.ContentToolResult:
				if b.ToolUseID == "" {
					continue
				}
			}
			cleanBlocks = append(cleanBlocks, b)
		}

		if len(cleanBlocks) == 0 {
			continue
		}

		msgs = append(msgs, api.Message{
			Role:    api.MessageRole(m.Role),
			Content: cleanBlocks,
		})
	}
	return msgs
}
