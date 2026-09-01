package bootstrap

import (
	"context"
	"testing"

	config "github.com/hieu-glaw/glaw-code/internal/modules/config/domain/entity"
	permentity "github.com/hieu-glaw/glaw-code/internal/modules/permission/domain/entity"
	permservice "github.com/hieu-glaw/glaw-code/internal/modules/permission/domain/service"
)

// =====================================================================
// LoadConfig Tests
// =====================================================================

func TestLoadConfigDefaults(t *testing.T) {
	dir := t.TempDir()
	settings, cfg, err := LoadConfig(dir, "", "", "")
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if settings.Model == "" {
		t.Error("Model should not be empty with defaults")
	}
	if cfg == nil {
		t.Fatal("Config should not be nil")
	}
	if cfg.MaxTokens != 16384 {
		t.Errorf("MaxTokens = %d, want 16384", cfg.MaxTokens)
	}
}

func TestLoadConfigWithOverrides(t *testing.T) {
	dir := t.TempDir()
	settings, _, err := LoadConfig(dir, "gpt-4o", "read_only", "")
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if settings.Model != "gpt-4o" {
		t.Errorf("Model = %q, want %q", settings.Model, "gpt-4o")
	}
	if settings.Permissions.Mode != "read_only" {
		t.Errorf("PermissionMode = %q, want %q", settings.Permissions.Mode, "read_only")
	}
}

func TestLoadConfigWithExplicitConfigPath(t *testing.T) {
	dir := t.TempDir()
	// Non-existent config path should not error (just warn)
	_, _, err := LoadConfig(dir, "", "", "/nonexistent/path/config.json")
	if err != nil {
		t.Fatalf("LoadConfig with bad path should not error: %v", err)
	}
}

// =====================================================================
// CreateAPIClient Tests
// =====================================================================

func TestCreateAPIClientMissingCredentials(t *testing.T) {
	// Clear any existing env vars
	keyVars := []string{
		"OPENROUTER_API_KEY", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN",
		"OPENAI_API_KEY", "GEMINI_API_KEY", "XAI_API_KEY", "GLAW_API_KEY",
	}
	for _, k := range keyVars {
		t.Setenv(k, "")
	}

	// The default model uses openrouter prefix which needs OPENROUTER_API_KEY
	_, err := CreateAPIClient("openrouter:test-model")
	if err == nil {
		t.Error("expected error for missing credentials")
	}
}

// =====================================================================
// ConvertMCPConfigs Tests
// =====================================================================

func TestConvertMCPConfigsNil(t *testing.T) {
	result := ConvertMCPConfigs(nil)
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestConvertMCPConfigsEmpty(t *testing.T) {
	result := ConvertMCPConfigs(map[string]*config.MCPServerConfig{})
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestConvertMCPConfigsPopulated(t *testing.T) {
	servers := map[string]*config.MCPServerConfig{
		"test-server": {
			Transport: "stdio",
			Command:   "node",
			Args:      []string{"server.js"},
			Env:       map[string]string{"KEY": "value"},
		},
	}
	result := ConvertMCPConfigs(servers)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	sc, ok := result["test-server"]
	if !ok {
		t.Fatal("expected test-server entry")
	}
	if sc.Transport != "stdio" {
		t.Errorf("Transport = %q, want %q", sc.Transport, "stdio")
	}
	if sc.Command != "node" {
		t.Errorf("Command = %q, want %q", sc.Command, "node")
	}
	if len(sc.Args) != 1 || sc.Args[0] != "server.js" {
		t.Errorf("Args = %v, want [server.js]", sc.Args)
	}
}

func TestConvertMCPConfigsNilValue(t *testing.T) {
	servers := map[string]*config.MCPServerConfig{
		"nil-server": nil,
	}
	result := ConvertMCPConfigs(servers)
	if len(result) != 0 {
		t.Errorf("nil config values should be skipped, got %d entries", len(result))
	}
}

func TestConvertMCPConfigsMultiple(t *testing.T) {
	servers := map[string]*config.MCPServerConfig{
		"server-a": {Transport: "sse", URL: "http://localhost:3000"},
		"server-b": {Transport: "stdio", Command: "python", Args: []string{"-m", "server"}},
	}
	result := ConvertMCPConfigs(servers)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	if result["server-a"].URL != "http://localhost:3000" {
		t.Errorf("server-a URL = %q", result["server-a"].URL)
	}
	if result["server-b"].Command != "python" {
		t.Errorf("server-b Command = %q", result["server-b"].Command)
	}
}

// =====================================================================
// SetupMCP Tests
// =====================================================================

func TestSetupMCPWithEmpty(t *testing.T) {
	ctx := context.Background()
	mgr := SetupMCP(ctx, nil)
	if mgr == nil {
		t.Error("expected non-nil MCP manager")
	}
}

// =====================================================================
// CreateRuntime Tests
// =====================================================================

func TestCreateRuntime(t *testing.T) {
	// We can't easily test with a real API client, but we can test the wiring
	// by passing nil (the runtime accepts nil client)
	settings, cfg, err := LoadConfig(t.TempDir(), "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = settings

	permMgr := permservice.NewPermissionManager(permentity.PermWorkspaceWrite, t.TempDir())
	rt := CreateRuntime(nil, cfg, permMgr, nil)
	if rt == nil {
		t.Fatal("expected non-nil runtime")
	}
	if rt.GetModel() != cfg.Model {
		t.Errorf("Model = %q, want %q", rt.GetModel(), cfg.Model)
	}
}
