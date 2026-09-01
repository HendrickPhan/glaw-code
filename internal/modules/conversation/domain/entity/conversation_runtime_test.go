package entity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	config "github.com/hieu-glaw/glaw-code/internal/modules/config/domain/entity"
	permentity "github.com/hieu-glaw/glaw-code/internal/modules/permission/domain/entity"
	permservice "github.com/hieu-glaw/glaw-code/internal/modules/permission/domain/service"
	sessionentity "github.com/hieu-glaw/glaw-code/internal/modules/session/domain/entity"
	"github.com/hieu-glaw/glaw-code/internal/shared/api"
)

func apiUsage(in, out int) api.Usage {
	return api.Usage{InputTokens: in, OutputTokens: out}
}

// --- Config tests ---

func TestDefaultConfig(t *testing.T) {
	c := DefaultConfig()
	if c.Model != "openrouter:nvidia/nemotron-3-super-120b-a12b:free" {
		t.Errorf("Model = %q", c.Model)
	}
	if c.MaxTokens != 16384 {
		t.Errorf("MaxTokens = %d", c.MaxTokens)
	}
	if c.PermissionMode != permentity.PermWorkspaceWrite {
		t.Errorf("PermissionMode = %q", c.PermissionMode)
	}
}

func TestConfigFromSettings(t *testing.T) {
	temp := 0.5
	s := config.Settings{
		Model:       "test-model",
		MaxTokens:   4096,
		Temperature: &temp,
	}
	c := ConfigFromSettings(s)
	if c.Model != "test-model" {
		t.Errorf("Model = %q", c.Model)
	}
	if c.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d", c.MaxTokens)
	}
}

func TestApplyOverrides(t *testing.T) {
	c := DefaultConfig()
	c.ApplyOverrides("claude-opus-4-6", "danger_full_access")
	if c.Model != "claude-opus-4-6" {
		t.Errorf("Model = %q", c.Model)
	}
	if c.PermissionMode != permentity.PermDangerFullAccess {
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
		PermissionMode: permentity.PermReadOnly,
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
	if !strings.Contains(result, "Go project") {
		t.Error("should contain project context")
	}
}

func TestBuildWithToolDescriptions(t *testing.T) {
	b := NewSystemPromptBuilder()
	b.ToolDescriptions = []string{"bash: execute commands", "read_file: read files"}
	result := b.Build()
	if !strings.Contains(result, "bash") {
		t.Error("should contain tool descriptions")
	}
}

// --- UsageTracker tests ---

func TestUsageTrackerRecord(t *testing.T) {
	tr := NewUsageTracker()
	tr.Record(apiUsage(10, 20))

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
	tr.Record(apiUsage(10, 20))
	tr.Record(apiUsage(5, 15))

	if tr.Cumulative.InputTokens != 15 {
		t.Errorf("Cumulative.InputTokens = %d, want 15", tr.Cumulative.InputTokens)
	}
	if tr.Cumulative.OutputTokens != 35 {
		t.Errorf("Cumulative.OutputTokens = %d, want 35", tr.Cumulative.OutputTokens)
	}
}

func TestEstimateCost(t *testing.T) {
	tr := NewUsageTracker()
	tr.Record(apiUsage(1_000_000, 1_000_000))

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
}

func TestFormatUSD(t *testing.T) {
	got := FormatUSD(1.5)
	if got != "$1.5000" {
		t.Errorf("FormatUSD(1.5) = %q, want %q", got, "$1.5000")
	}
}

// --- ConversationRuntime tests ---

func TestNewConversationRuntime(t *testing.T) {
	rt := NewConversationRuntime(nil, DefaultConfig(), sessionentity.NewSession(), permservice.NewPermissionManager(permentity.PermReadOnly, "/tmp"), nil)
	if rt == nil {
		t.Fatal("runtime should not be nil")
	}
}

func TestRuntimeGetSetModel(t *testing.T) {
	rt := NewConversationRuntime(nil, DefaultConfig(), sessionentity.NewSession(), permservice.NewPermissionManager(permentity.PermReadOnly, "/tmp"), nil)
	if rt.GetModel() != "openrouter:nvidia/nemotron-3-super-120b-a12b:free" {
		t.Errorf("Model = %q", rt.GetModel())
	}
	rt.SetModel("claude-opus-4-6")
	if rt.GetModel() != "claude-opus-4-6" {
		t.Errorf("Model = %q", rt.GetModel())
	}
}

func TestRuntimeGetSetPermissionMode(t *testing.T) {
	rt := NewConversationRuntime(nil, DefaultConfig(), sessionentity.NewSession(), permservice.NewPermissionManager(permentity.PermReadOnly, "/tmp"), nil)
	if rt.GetPermissionMode() != "read_only" {
		t.Errorf("PermMode = %q", rt.GetPermissionMode())
	}
	rt.SetPermissionMode("danger_full_access")
	if rt.GetPermissionMode() != "danger_full_access" {
		t.Errorf("PermMode = %q", rt.GetPermissionMode())
	}
}

func TestRuntimeGetSessionID(t *testing.T) {
	s := sessionentity.NewSession()
	rt := NewConversationRuntime(nil, DefaultConfig(), s, permservice.NewPermissionManager(permentity.PermReadOnly, "/tmp"), nil)
	if rt.GetSessionID() != s.ID {
		t.Errorf("SessionID = %q, want %q", rt.GetSessionID(), s.ID)
	}
}

func TestRuntimeCompactSession(t *testing.T) {
	s := sessionentity.NewSession()
	for i := 0; i < 25; i++ {
		s.AddUserMessageFromText("msg")
	}
	rt := NewConversationRuntime(nil, DefaultConfig(), s, permservice.NewPermissionManager(permentity.PermReadOnly, "/tmp"), nil)
	if err := rt.CompactSession(); err != nil {
		t.Fatal(err)
	}
	if s.MessageCount() != 20 {
		t.Errorf("after compact: %d messages, want 20", s.MessageCount())
	}
}

func TestRuntimeClearSession(t *testing.T) {
	s := sessionentity.NewSession()
	s.AddUserMessageFromText("hello")
	rt := NewConversationRuntime(nil, DefaultConfig(), s, permservice.NewPermissionManager(permentity.PermReadOnly, "/tmp"), nil)
	rt.ClearSession()
	if s.MessageCount() != 0 {
		t.Errorf("after clear: %d messages, want 0", s.MessageCount())
	}
}

// --- Permission check tests (basic) ---

func TestPermissionCheck(t *testing.T) {
	tests := []struct {
		mode permentity.PermissionMode
		perm permentity.Permission
		want bool
	}{
		{permentity.PermDangerFullAccess, permentity.PermExecuteCommand, true},
		{permentity.PermAllow, permentity.PermReadFile, true},
		{permentity.PermReadOnly, permentity.PermReadFile, true},
		{permentity.PermReadOnly, permentity.PermWriteFile, false},
		{permentity.PermReadOnly, permentity.PermExecuteCommand, false},
		{permentity.PermReadOnly, permentity.PermNetwork, true},
		{permentity.PermWorkspaceWrite, permentity.PermReadFile, true},
		{permentity.PermWorkspaceWrite, permentity.PermWriteFile, true},
		{permentity.PermWorkspaceWrite, permentity.PermExecuteCommand, false},
		{permentity.PermPrompt, permentity.PermExecuteCommand, true},
	}
	for _, tt := range tests {
		m := permservice.NewPermissionManager(tt.mode, "/tmp")
		got := m.Check(tt.perm)
		if got != tt.want {
			t.Errorf("Check(%q, %q) = %v, want %v", tt.mode, tt.perm, got, tt.want)
		}
	}
}

func TestCheckTool(t *testing.T) {
	tests := []struct {
		mode    permentity.PermissionMode
		require permentity.PermissionMode
		want    bool
	}{
		{permentity.PermDangerFullAccess, permentity.PermDangerFullAccess, true},
		{permentity.PermReadOnly, permentity.PermReadOnly, true},
		{permentity.PermReadOnly, permentity.PermDangerFullAccess, false},
		{permentity.PermWorkspaceWrite, permentity.PermDangerFullAccess, false},
		{permentity.PermWorkspaceWrite, permentity.PermWorkspaceWrite, true},
		{permentity.PermPrompt, permentity.PermDangerFullAccess, true},
	}
	for _, tt := range tests {
		m := permservice.NewPermissionManager(tt.mode, "/tmp")
		got := m.CheckTool("test", tt.require)
		if got != tt.want {
			t.Errorf("CheckTool(%q, %q) = %v, want %v", tt.mode, tt.require, got, tt.want)
		}
	}
}
