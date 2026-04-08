package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/hieu-glaw/glaw-code/internal/api"
)

// --- Session tests ---

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

	// Tool results should be added as a new user message (per Anthropic API spec)
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

// --- Persistence tests ---

func TestSaveAndLoadSession(t *testing.T) {
	dir := t.TempDir()
	s := NewSession()
	s.AddUserMessageFromText("hello")
	s.AddAssistantMessage([]api.ContentBlock{api.NewTextBlock("world")}, &api.Usage{InputTokens: 5, OutputTokens: 10})

	path, err := SaveSession(s, dir)
	if err != nil {
		t.Fatalf("SaveSession error: %v", err)
	}

	loaded, err := LoadSession(path)
	if err != nil {
		t.Fatalf("LoadSession error: %v", err)
	}
	if loaded.ID != s.ID {
		t.Errorf("ID = %q, want %q", loaded.ID, s.ID)
	}
	if loaded.Version != s.Version {
		t.Errorf("Version = %d, want %d", loaded.Version, s.Version)
	}
	if loaded.MessageCount() != 2 {
		t.Errorf("MessageCount = %d, want 2", loaded.MessageCount())
	}
}

func TestLoadSessionNotFound(t *testing.T) {
	_, err := LoadSession("nonexistent.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

// --- Permission tests ---

func TestPermissionCheck(t *testing.T) {
	tests := []struct {
		mode PermissionMode
		perm Permission
		want bool
	}{
		{PermDangerFullAccess, PermExecuteCommand, true},
		{PermAllow, PermReadFile, true},
		{PermReadOnly, PermReadFile, true},
		{PermReadOnly, PermWriteFile, false},
		{PermReadOnly, PermExecuteCommand, false},
		{PermReadOnly, PermNetwork, true},
		{PermWorkspaceWrite, PermReadFile, true},
		{PermWorkspaceWrite, PermWriteFile, true},
		{PermWorkspaceWrite, PermExecuteCommand, false},
		{PermPrompt, PermExecuteCommand, true},
	}
	for _, tt := range tests {
		m := NewPermissionManager(tt.mode, "/tmp")
		got := m.Check(tt.perm)
		if got != tt.want {
			t.Errorf("Check(%q, %q) = %v, want %v", tt.mode, tt.perm, got, tt.want)
		}
	}
}

func TestCheckTool(t *testing.T) {
	tests := []struct {
		mode    PermissionMode
		require PermissionMode
		want    bool
	}{
		{PermDangerFullAccess, PermDangerFullAccess, true},
		{PermReadOnly, PermReadOnly, true},
		{PermReadOnly, PermDangerFullAccess, false},
		{PermWorkspaceWrite, PermDangerFullAccess, false},
		{PermWorkspaceWrite, PermWorkspaceWrite, true},
		{PermPrompt, PermDangerFullAccess, true},
	}
	for _, tt := range tests {
		m := NewPermissionManager(tt.mode, "/tmp")
		got := m.CheckTool("test", tt.require)
		if got != tt.want {
			t.Errorf("CheckTool(%q, %q) = %v, want %v", tt.mode, tt.require, got, tt.want)
		}
	}
}

// --- UsageTracker tests ---

func TestUsageTrackerRecord(t *testing.T) {
	tr := NewUsageTracker()
	tr.Record(api.Usage{InputTokens: 10, OutputTokens: 20})

	if tr.LatestTurn.InputTokens != 10 {
		t.Errorf("LatestTurn.InputTokens = %d", tr.LatestTurn.InputTokens)
	}
	if tr.Cumulative.InputTokens != 10 {
		t.Errorf("Cumulative.InputTokens = %d", tr.Cumulative.InputTokens)
	}
	if tr.Turns != 1 {
		t.Errorf("Turns = %d, want 1", tr.Turns)
	}
}

func TestUsageTrackerMultipleRecords(t *testing.T) {
	tr := NewUsageTracker()
	tr.Record(api.Usage{InputTokens: 10, OutputTokens: 20})
	tr.Record(api.Usage{InputTokens: 5, OutputTokens: 15})

	if tr.Cumulative.InputTokens != 15 {
		t.Errorf("Cumulative.InputTokens = %d, want 15", tr.Cumulative.InputTokens)
	}
	if tr.Cumulative.OutputTokens != 35 {
		t.Errorf("Cumulative.OutputTokens = %d, want 35", tr.Cumulative.OutputTokens)
	}
	if tr.Turns != 2 {
		t.Errorf("Turns = %d, want 2", tr.Turns)
	}
}

func TestEstimateCost(t *testing.T) {
	tr := NewUsageTracker()
	tr.Record(api.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000})

	in, out, total := tr.EstimateCost("claude-sonnet-4-6")
	if in == 0 {
		t.Error("input cost should not be 0")
	}
	if out == 0 {
		t.Error("output cost should not be 0")
	}
	if total != in+out {
		t.Errorf("total = %v, want %v + %v = %v", total, in, out, in+out)
	}
}

func TestPricingForModel(t *testing.T) {
	haiku := PricingForModel("claude-haiku-4-5")
	if haiku.InputCostPerMillion != 1.0 {
		t.Errorf("haiku input = %v, want 1.0", haiku.InputCostPerMillion)
	}

	opus := PricingForModel("claude-opus-4-6")
	if opus.InputCostPerMillion != 15.0 {
		t.Errorf("opus input = %v, want 15.0", opus.InputCostPerMillion)
	}

	sonnet := PricingForModel("claude-sonnet-4-6")
	if sonnet.InputCostPerMillion != 3.0 {
		t.Errorf("sonnet input = %v, want 3.0", sonnet.InputCostPerMillion)
	}

	// Non-Anthropic models
	gpt4o := PricingForModel("gpt-4o")
	if gpt4o.InputCostPerMillion != 2.5 {
		t.Errorf("gpt-4o input = %v, want 2.5", gpt4o.InputCostPerMillion)
	}

	gemini := PricingForModel("gemini-2.5-pro")
	if gemini.InputCostPerMillion != 1.25 {
		t.Errorf("gemini-2.5-pro input = %v, want 1.25", gemini.InputCostPerMillion)
	}

	ollama := PricingForModel("ollama:llama3")
	if ollama.InputCostPerMillion != 0 {
		t.Errorf("ollama input = %v, want 0", ollama.InputCostPerMillion)
	}
}

// --- Config tests ---

func TestDefaultConfig(t *testing.T) {
	c := DefaultConfig()
	if c.Model != "claude-sonnet-4-6" {
		t.Errorf("Model = %q", c.Model)
	}
	if c.MaxTokens != 16384 {
		t.Errorf("MaxTokens = %d", c.MaxTokens)
	}
	if c.PermissionMode != PermWorkspaceWrite {
		t.Errorf("PermissionMode = %q", c.PermissionMode)
	}
}

func TestApplyOverrides(t *testing.T) {
	c := DefaultConfig()
	c.ApplyOverrides("claude-opus-4-6", "danger_full_access")
	if c.Model != "claude-opus-4-6" {
		t.Errorf("Model = %q", c.Model)
	}
	if c.PermissionMode != PermDangerFullAccess {
		t.Errorf("PermissionMode = %q", c.PermissionMode)
	}
}

func TestApplyOverridesEmpty(t *testing.T) {
	c := DefaultConfig()
	original := c.Model
	c.ApplyOverrides("", "")
	if c.Model != original {
		t.Error("empty overrides should not change config")
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Model:          "test-model",
		MaxTokens:      4096,
		Temperature:    0.7,
		PermissionMode: PermReadOnly,
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if loaded.Model != "test-model" {
		t.Errorf("Model = %q", loaded.Model)
	}
	if loaded.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d", loaded.MaxTokens)
	}
}

// --- SystemPromptBuilder tests ---

func TestBuildEmpty(t *testing.T) {
	b := NewSystemPromptBuilder()
	result := b.Build()
	if result == "" {
		t.Error("Build() should not be empty")
	}
}

func TestBuildWithProjectContext(t *testing.T) {
	b := NewSystemPromptBuilder()
	b.ProjectContext = "Go project with 10 packages"
	result := b.Build()
	if !contains(result, "Go project") {
		t.Error("should contain project context")
	}
}

func TestBuildWithToolDescriptions(t *testing.T) {
	b := NewSystemPromptBuilder()
	b.ToolDescriptions = []string{"bash: execute commands", "read_file: read files"}
	result := b.Build()
	if !contains(result, "bash") {
		t.Error("should contain tool descriptions")
	}
}

// --- ConversationRuntime tests ---

func TestNewConversationRuntime(t *testing.T) {
	rt := NewConversationRuntime(nil, DefaultConfig(), NewSession(), NewPermissionManager(PermReadOnly, "/tmp"), nil)
	if rt == nil {
		t.Fatal("runtime should not be nil")
	}
}

func TestRuntimeGetSetModel(t *testing.T) {
	rt := NewConversationRuntime(nil, DefaultConfig(), NewSession(), NewPermissionManager(PermReadOnly, "/tmp"), nil)
	if rt.GetModel() != "claude-sonnet-4-6" {
		t.Errorf("Model = %q", rt.GetModel())
	}
	rt.SetModel("claude-opus-4-6")
	if rt.GetModel() != "claude-opus-4-6" {
		t.Errorf("Model = %q", rt.GetModel())
	}
}

func TestRuntimeGetSetPermissionMode(t *testing.T) {
	rt := NewConversationRuntime(nil, DefaultConfig(), NewSession(), NewPermissionManager(PermReadOnly, "/tmp"), nil)
	if rt.GetPermissionMode() != "read_only" {
		t.Errorf("PermMode = %q", rt.GetPermissionMode())
	}
	rt.SetPermissionMode("danger_full_access")
	if rt.GetPermissionMode() != "danger_full_access" {
		t.Errorf("PermMode = %q", rt.GetPermissionMode())
	}
}

func TestRuntimeGetSessionID(t *testing.T) {
	s := NewSession()
	rt := NewConversationRuntime(nil, DefaultConfig(), s, NewPermissionManager(PermReadOnly, "/tmp"), nil)
	if rt.GetSessionID() != s.ID {
		t.Errorf("SessionID = %q, want %q", rt.GetSessionID(), s.ID)
	}
}

func TestRuntimeCompactSession(t *testing.T) {
	s := NewSession()
	for i := 0; i < 25; i++ {
		s.AddUserMessageFromText("msg")
	}
	rt := NewConversationRuntime(nil, DefaultConfig(), s, NewPermissionManager(PermReadOnly, "/tmp"), nil)
	if err := rt.CompactSession(); err != nil {
		t.Fatal(err)
	}
	if s.MessageCount() != 20 {
		t.Errorf("after compact: %d messages, want 20", s.MessageCount())
	}
}

func TestRuntimeClearSession(t *testing.T) {
	s := NewSession()
	s.AddUserMessageFromText("hello")
	rt := NewConversationRuntime(nil, DefaultConfig(), s, NewPermissionManager(PermReadOnly, "/tmp"), nil)
	rt.ClearSession()
	if s.MessageCount() != 0 {
		t.Errorf("after clear: %d messages, want 0", s.MessageCount())
	}
}

func TestFormatUSD(t *testing.T) {
	got := FormatUSD(1.5)
	if got != "$1.5000" {
		t.Errorf("FormatUSD(1.5) = %q, want %q", got, "$1.5000")
	}
}
