package commands

import (
	"context"
	"strings"
	"testing"
)

// mockRuntime implements the Runtime interface for testing.
type mockRuntime struct {
	model     string
	permMode  string
	sessionID string
	msgCount  int
	usage     UsageInfo
}

func (m *mockRuntime) GetModel() string         { return m.model }
func (m *mockRuntime) SetModel(s string)         { m.model = s }
func (m *mockRuntime) GetPermissionMode() string { return m.permMode }
func (m *mockRuntime) SetPermissionMode(s string) { m.permMode = s }
func (m *mockRuntime) GetMessageCount() int       { return m.msgCount }
func (m *mockRuntime) GetSessionID() string       { return m.sessionID }
func (m *mockRuntime) GetUsageInfo() UsageInfo    { return m.usage }
func (m *mockRuntime) CompactSession() error      { return nil }
func (m *mockRuntime) ClearSession()              { m.msgCount = 0 }
func (m *mockRuntime) RunGitCommand(args ...string) (string, error) {
	if len(args) > 0 && args[0] == "diff" {
		return "fake diff output", nil
	}
	return "", nil
}
func (m *mockRuntime) GetWorkspaceRoot() string                { return "/tmp" }
func (m *mockRuntime) GetAllSettings() map[string]interface{}  { return map[string]interface{}{"model": m.model} }
func (m *mockRuntime) SetConfigValue(key, value string) error  { return nil }
func (m *mockRuntime) GetLSPStatus() []LSPServerStatus         { return nil }
func (m *mockRuntime) RevertLastTurn() (int, error)            { return 0, nil }
func (m *mockRuntime) RevertAll() (int, error)                 { return 0, nil }

func newMockRuntime() *mockRuntime {
	return &mockRuntime{
		model:     "claude-sonnet-4-6",
		permMode:  "workspace_write",
		sessionID: "sess_test",
		msgCount:  5,
		usage:     UsageInfo{InputTokens: 100, OutputTokens: 50, TotalCostUSD: 0.05},
	}
}

// --- Parsing tests ---

func TestParseHelp(t *testing.T) {
	parsed, _ := Parse("/help")
	if parsed == nil {
		t.Fatal("expected parsed command")
		return
	}
	if parsed.Spec.Name != "help" {
		t.Errorf("Name = %q, want %q", parsed.Spec.Name, "help")
	}
}

func TestParseWithAlias(t *testing.T) {
	for _, input := range []string{"/h", "/?"} {
		parsed, _ := Parse(input)
		if parsed == nil {
			t.Fatalf("Parse(%q) returned nil", input)
			return
		}
		if parsed.Spec.Name != "help" {
			t.Errorf("Parse(%q).Name = %q, want %q", input, parsed.Spec.Name, "help")
		}
	}
}

func TestParseWithArgument(t *testing.T) {
	parsed, _ := Parse("/model claude-opus-4-6")
	if parsed == nil {
		t.Fatal("expected parsed command")
		return
	}
	if parsed.Spec.Name != "model" {
		t.Errorf("Name = %q", parsed.Spec.Name)
	}
	if parsed.Remainder != "claude-opus-4-6" {
		t.Errorf("Remainder = %q, want %q", parsed.Remainder, "claude-opus-4-6")
	}
}

func TestParseNonCommand(t *testing.T) {
	parsed, _ := Parse("hello world")
	if parsed != nil {
		t.Error("expected nil for non-command")
	}
}

func TestParseUnknownCommand(t *testing.T) {
	parsed, _ := Parse("/xyzabc123")
	if parsed != nil {
		t.Error("expected nil for unknown command")
	}
}

// --- Suggestion tests ---

func TestSuggestCommandsExact(t *testing.T) {
	suggestions := SuggestCommands("/help")
	if len(suggestions) == 0 {
		t.Fatal("expected suggestions for 'help'")
	}
	found := false
	for _, s := range suggestions {
		if s.Name == "help" {
			found = true
		}
	}
	if !found {
		t.Error("should suggest 'help'")
	}
}

func TestSuggestCommandsFuzzy(t *testing.T) {
	suggestions := SuggestCommands("/hlep")
	found := false
	for _, s := range suggestions {
		if s.Name == "help" {
			found = true
		}
	}
	if !found {
		t.Error("should fuzzy-match 'hlep' to 'help'")
	}
}

func TestSuggestCommandsNoMatch(t *testing.T) {
	suggestions := SuggestCommands("/xyzqwerty")
	if len(suggestions) > 0 {
		t.Errorf("expected no suggestions, got %d", len(suggestions))
	}
}

// --- Dispatcher tests ---

func TestHandleHelp(t *testing.T) {
	d := NewDispatcher(newMockRuntime())
	result, err := d.Handle(context.Background(), "/help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Message, "Available Commands") {
		t.Errorf("Message = %q", result.Message)
	}
	if result.Action != "continue" {
		t.Errorf("Action = %q", result.Action)
	}
}

func TestHandleStatus(t *testing.T) {
	d := NewDispatcher(newMockRuntime())
	result, err := d.Handle(context.Background(), "/status")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Message, "claude-sonnet-4-6") {
		t.Errorf("should contain model: %q", result.Message)
	}
	if !strings.Contains(result.Message, "sess_test") {
		t.Errorf("should contain session ID: %q", result.Message)
	}
}

func TestHandleVersion(t *testing.T) {
	d := NewDispatcher(newMockRuntime())
	result, err := d.Handle(context.Background(), "/version")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Message, "v1.0.0") {
		t.Errorf("Message = %q", result.Message)
	}
}

func TestHandleModelGet(t *testing.T) {
	d := NewDispatcher(newMockRuntime())
	result, err := d.Handle(context.Background(), "/model")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Message, "claude-sonnet-4-6") {
		t.Errorf("Message = %q", result.Message)
	}
}

func TestHandleModelSet(t *testing.T) {
	m := newMockRuntime()
	d := NewDispatcher(m)
	result, err := d.Handle(context.Background(), "/model claude-opus-4-6")
	if err != nil {
		t.Fatal(err)
	}
	if m.model != "claude-opus-4-6" {
		t.Errorf("model = %q, want %q", m.model, "claude-opus-4-6")
	}
	if !strings.Contains(result.Message, "claude-opus-4-6") {
		t.Errorf("Message = %q", result.Message)
	}
}

func TestHandlePermissionsGet(t *testing.T) {
	d := NewDispatcher(newMockRuntime())
	result, err := d.Handle(context.Background(), "/permissions")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Message, "workspace_write") {
		t.Errorf("Message = %q", result.Message)
	}
}

func TestHandlePermissionsSet(t *testing.T) {
	m := newMockRuntime()
	d := NewDispatcher(m)
	result, err := d.Handle(context.Background(), "/perm read_only")
	if err != nil {
		t.Fatal(err)
	}
	if m.permMode != "read_only" {
		t.Errorf("permMode = %q", m.permMode)
	}
	if !strings.Contains(result.Message, "read_only") {
		t.Errorf("Message = %q", result.Message)
	}
}

func TestHandleClear(t *testing.T) {
	m := newMockRuntime()
	d := NewDispatcher(m)
	result, err := d.Handle(context.Background(), "/clear")
	if err != nil {
		t.Fatal(err)
	}
	if m.msgCount != 0 {
		t.Errorf("msgCount = %d, want 0", m.msgCount)
	}
	if !strings.Contains(result.Message, "cleared") {
		t.Errorf("Message = %q", result.Message)
	}
}

func TestHandleCost(t *testing.T) {
	d := NewDispatcher(newMockRuntime())
	result, err := d.Handle(context.Background(), "/cost")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Message, "100") {
		t.Errorf("should show input tokens: %q", result.Message)
	}
	if !strings.Contains(result.Message, "0.05") {
		t.Errorf("should show cost: %q", result.Message)
	}
}

func TestHandleCompact(t *testing.T) {
	d := NewDispatcher(newMockRuntime())
	result, err := d.Handle(context.Background(), "/compact")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Message, "compacted") {
		t.Errorf("Message = %q", result.Message)
	}
}

func TestHandleDiff(t *testing.T) {
	d := NewDispatcher(newMockRuntime())
	result, err := d.Handle(context.Background(), "/diff")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Message, "fake diff") {
		t.Errorf("Message = %q", result.Message)
	}
}

func TestHandleUnknownCommand(t *testing.T) {
	d := NewDispatcher(newMockRuntime())
	result, err := d.Handle(context.Background(), "/xyzabc123456")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Message, "Unknown command") {
		t.Errorf("Message = %q", result.Message)
	}
}

func TestHandleVersionAlias(t *testing.T) {
	d := NewDispatcher(newMockRuntime())
	result, err := d.Handle(context.Background(), "/v")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Message, "v1.0.0") {
		t.Errorf("Message = %q", result.Message)
	}
}

// --- Levenshtein tests ---

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "abc", 3},
		{"abc", "", 3},
		{"abc", "abc", 0},
		{"kitten", "sitting", 3},
		{"help", "hlep", 2},
		{"a", "b", 1},
	}
	for _, tt := range tests {
		got := levenshtein(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
