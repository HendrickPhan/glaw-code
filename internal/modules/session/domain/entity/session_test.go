package entity

import (
	"sync"
	"testing"

	"github.com/hieu-glaw/glaw-code/internal/shared/api"
)

func TestNewSession(t *testing.T) {
	s := NewSession()
	if s.Version != 1 {
		t.Errorf("Version = %d, want 1", s.Version)
	}
	if s.ID == "" {
		t.Error("ID should not be empty")
	}
	if len(s.Messages) != 0 {
		t.Errorf("Messages = %d, want 0", len(s.Messages))
	}
}

func TestAddUserMessage(t *testing.T) {
	s := NewSession()
	blocks := []api.ContentBlock{api.NewTextBlock("hello")}
	s.AddUserMessage(blocks)

	if len(s.Messages) != 1 {
		t.Fatalf("Messages = %d, want 1", len(s.Messages))
	}
	if s.Messages[0].Role != string(api.RoleUser) {
		t.Errorf("Role = %q, want %q", s.Messages[0].Role, api.RoleUser)
	}
	if s.Messages[0].Blocks[0].Text != "hello" {
		t.Errorf("Text = %q, want %q", s.Messages[0].Blocks[0].Text, "hello")
	}
}

func TestAddUserMessageFromText(t *testing.T) {
	s := NewSession()
	s.AddUserMessageFromText("hi there")

	if len(s.Messages) != 1 {
		t.Fatalf("Messages = %d, want 1", len(s.Messages))
	}
	if s.Messages[0].Blocks[0].Text != "hi there" {
		t.Errorf("Text = %q", s.Messages[0].Blocks[0].Text)
	}
}

func TestAddAssistantMessage(t *testing.T) {
	s := NewSession()
	blocks := []api.ContentBlock{api.NewTextBlock("response")}
	usage := &api.Usage{InputTokens: 10, OutputTokens: 20}
	s.AddAssistantMessage(blocks, usage)

	if len(s.Messages) != 1 {
		t.Fatalf("Messages = %d, want 1", len(s.Messages))
	}
	if s.Messages[0].Role != string(api.RoleAssistant) {
		t.Errorf("Role = %q", s.Messages[0].Role)
	}
	if s.Messages[0].Usage.InputTokens != 10 {
		t.Errorf("Usage.InputTokens = %d, want 10", s.Messages[0].Usage.InputTokens)
	}
}

func TestAddToolResult(t *testing.T) {
	s := NewSession()
	s.AddUserMessageFromText("test")
	s.AddAssistantMessage([]api.ContentBlock{api.NewTextBlock("thinking...")}, nil)
	s.AddToolResult("tool_1", "result data", false)

	last := s.Messages[len(s.Messages)-1]
	if last.Role != string(api.RoleUser) {
		t.Fatalf("expected last message to be user (for tool_result), got %q", last.Role)
	}
	found := false
	for _, b := range last.Blocks {
		if b.Type == api.ContentToolResult {
			found = true
			if b.ToolUseID != "tool_1" {
				t.Errorf("ToolUseID = %q, want %q", b.ToolUseID, "tool_1")
			}
		}
	}
	if !found {
		t.Error("tool result block not found")
	}
}

func TestMessageCount(t *testing.T) {
	s := NewSession()
	if s.MessageCount() != 0 {
		t.Errorf("count = %d, want 0", s.MessageCount())
	}
	s.AddUserMessageFromText("a")
	s.AddUserMessageFromText("b")
	if s.MessageCount() != 2 {
		t.Errorf("count = %d, want 2", s.MessageCount())
	}
}

func TestAsAPIMessages(t *testing.T) {
	s := NewSession()
	s.AddUserMessageFromText("hello")
	s.AddAssistantMessage([]api.ContentBlock{api.NewTextBlock("hi")}, nil)

	msgs := s.AsAPIMessages()
	if len(msgs) != 2 {
		t.Fatalf("len = %d, want 2", len(msgs))
	}
	if msgs[0].Role != api.RoleUser {
		t.Errorf("Role[0] = %q", msgs[0].Role)
	}
	if msgs[1].Role != api.RoleAssistant {
		t.Errorf("Role[1] = %q", msgs[1].Role)
	}
}

func TestSessionConcurrency(t *testing.T) {
	s := NewSession()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.AddUserMessageFromText("msg")
		}()
	}
	wg.Wait()
	if s.MessageCount() != 100 {
		t.Errorf("count = %d, want 100", s.MessageCount())
	}
}
