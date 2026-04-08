package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultSettings(t *testing.T) {
	s := DefaultSettings()
	if s.Model != "openrouter:nvidia/nemotron-3-super-120b-a12b:free" {
		t.Errorf("Model = %q, want %q", s.Model, "openrouter:nvidia/nemotron-3-super-120b-a12b:free")
	}
	if s.MaxTokens != 16384 {
		t.Errorf("MaxTokens = %d, want 16384", s.MaxTokens)
	}
	if s.Temperature == nil || *s.Temperature != 1.0 {
		t.Error("Temperature should be 1.0")
	}
	if s.Permissions.Mode != "workspace_write" {
		t.Errorf("Permissions.Mode = %q, want %q", s.Permissions.Mode, "workspace_write")
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, SettingsFileName)

	s := Settings{
		Model:     "claude-opus-4-6",
		MaxTokens: 8192,
	}
	data, _ := json.MarshalIndent(s, "", "  ")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	loaded, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile error: %v", err)
	}
	if loaded.Model != "claude-opus-4-6" {
		t.Errorf("Model = %q", loaded.Model)
	}
	if loaded.MaxTokens != 8192 {
		t.Errorf("MaxTokens = %d", loaded.MaxTokens)
	}
}

func TestLoadFromFileNotFound(t *testing.T) {
	loaded, err := LoadFromFile("/nonexistent/settings.json")
	if err != nil {
		t.Errorf("expected no error for missing file, got: %v", err)
	}
	if loaded != nil {
		t.Error("expected nil for missing file")
	}
}

func TestLoadFromFileInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, SettingsFileName)
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	_, err := LoadFromFile(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestMerge(t *testing.T) {
	base := DefaultSettings()

	overlay := Settings{
		Model:     "claude-opus-4-6",
		MaxTokens: 32768,
	}
	merged := Merge(base, overlay)

	if merged.Model != "claude-opus-4-6" {
		t.Errorf("Model = %q, want %q", merged.Model, "claude-opus-4-6")
	}
	if merged.MaxTokens != 32768 {
		t.Errorf("MaxTokens = %d, want 32768", merged.MaxTokens)
	}
	// Should keep base values for unset fields
	if merged.Permissions.Mode != "workspace_write" {
		t.Errorf("Permissions.Mode should be preserved from base")
	}
}

func TestMergePermissions(t *testing.T) {
	base := DefaultSettings()

	overlay := Settings{
		Permissions: PermissionSettings{
			Mode:  "read_only",
			Allow: []string{"read_file"},
			Deny:  []string{"execute_command"},
		},
	}
	merged := Merge(base, overlay)

	if merged.Permissions.Mode != "read_only" {
		t.Errorf("Permissions.Mode = %q", merged.Permissions.Mode)
	}
	if len(merged.Permissions.Allow) != 1 || merged.Permissions.Allow[0] != "read_file" {
		t.Errorf("Permissions.Allow = %v", merged.Permissions.Allow)
	}
	if len(merged.Permissions.Deny) != 1 || merged.Permissions.Deny[0] != "execute_command" {
		t.Errorf("Permissions.Deny = %v", merged.Permissions.Deny)
	}
}

func TestMergeEmptyOverlay(t *testing.T) {
	base := DefaultSettings()
	overlay := Settings{}
	merged := Merge(base, overlay)

	if merged.Model != base.Model {
		t.Errorf("empty overlay should not change Model")
	}
	if merged.MaxTokens != base.MaxTokens {
		t.Errorf("empty overlay should not change MaxTokens")
	}
}

func TestMergePlugins(t *testing.T) {
	base := DefaultSettings()
	overlay := Settings{
		Plugins: PluginSettings{
			Enabled:  []string{"mcp"},
			Disabled: []string{"legacy"},
		},
	}
	merged := Merge(base, overlay)

	if len(merged.Plugins.Enabled) != 1 || merged.Plugins.Enabled[0] != "mcp" {
		t.Errorf("Plugins.Enabled = %v", merged.Plugins.Enabled)
	}
	if len(merged.Plugins.Disabled) != 1 || merged.Plugins.Disabled[0] != "legacy" {
		t.Errorf("Plugins.Disabled = %v", merged.Plugins.Disabled)
	}
}

func TestMergeTemperature(t *testing.T) {
	base := DefaultSettings()
	newTemp := 0.5
	overlay := Settings{Temperature: &newTemp}
	merged := Merge(base, overlay)

	if merged.Temperature == nil || *merged.Temperature != 0.5 {
		t.Errorf("Temperature = %v, want 0.5", merged.Temperature)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, SettingsFileName)

	s := Settings{
		Model:       "grok-3",
		MaxTokens:   4096,
		APIBaseURL:  "https://api.x.ai/v1",
		Permissions: PermissionSettings{Mode: "danger_full_access"},
	}

	if err := Save(path, s); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	loaded, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile error: %v", err)
	}
	if loaded.Model != s.Model {
		t.Errorf("Model = %q, want %q", loaded.Model, s.Model)
	}
	if loaded.MaxTokens != s.MaxTokens {
		t.Errorf("MaxTokens = %d, want %d", loaded.MaxTokens, s.MaxTokens)
	}
	if loaded.APIBaseURL != s.APIBaseURL {
		t.Errorf("APIBaseURL = %q, want %q", loaded.APIBaseURL, s.APIBaseURL)
	}
	if loaded.Permissions.Mode != s.Permissions.Mode {
		t.Errorf("Permissions.Mode = %q, want %q", loaded.Permissions.Mode, s.Permissions.Mode)
	}
}

func TestLoadAllLayering(t *testing.T) {
	workspace := t.TempDir()

	// Create a fake home dir
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Write global settings
	globalDir := filepath.Join(home, DefaultGlobalDir)
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}
	global := Settings{Model: "global-model", MaxTokens: 9999}
	data, _ := json.MarshalIndent(global, "", "  ")
	if err := os.WriteFile(filepath.Join(globalDir, SettingsFileName), data, 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	// Write project settings
	projectDir := filepath.Join(workspace, ProjectDir)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}
	project := Settings{Model: "project-model"}
	data, _ = json.MarshalIndent(project, "", "  ")
	if err := os.WriteFile(filepath.Join(projectDir, SettingsFileName), data, 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	loaded, err := LoadAll(workspace)
	if err != nil {
		t.Fatalf("LoadAll error: %v", err)
	}

	// Project overrides global
	if loaded.Model != "project-model" {
		t.Errorf("Model = %q, want %q (project override)", loaded.Model, "project-model")
	}
	// Global value preserved when project doesn't override
	if loaded.MaxTokens != 9999 {
		t.Errorf("MaxTokens = %d, want 9999 (from global)", loaded.MaxTokens)
	}
}

func TestLoadAllNoFiles(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Clear model env so defaults are used
	t.Setenv("ANTHROPIC_DEFAULT_SONNET_MODEL", "")

	loaded, err := LoadAll(workspace)
	if err != nil {
		t.Fatalf("LoadAll error: %v", err)
	}
	// Should get the new default model
	if loaded.Model != "openrouter:nvidia/nemotron-3-super-120b-a12b:free" {
		t.Errorf("Model = %q, want default", loaded.Model)
	}
}

func TestProjectSettingsPath(t *testing.T) {
	p := ProjectSettingsPath("/workspace")
	expected := filepath.Join("/workspace", ProjectDir, SettingsFileName)
	if p != expected {
		t.Errorf("ProjectSettingsPath = %q, want %q", p, expected)
	}
}

func TestEnsureGlobalDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir, err := EnsureGlobalDir()
	if err != nil {
		t.Fatalf("EnsureGlobalDir error: %v", err)
	}

	expected := filepath.Join(home, DefaultGlobalDir)
	if dir != expected {
		t.Errorf("dir = %q, want %q", dir, expected)
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("directory was not created")
	}
}

func TestMergeEnv(t *testing.T) {
	base := DefaultSettings()
	overlay := Settings{
		Env: map[string]string{
			"ANTHROPIC_AUTH_TOKEN": "test-key",
			"ANTHROPIC_BASE_URL":   "https://api.test.com",
		},
	}
	merged := Merge(base, overlay)

	if merged.Env["ANTHROPIC_AUTH_TOKEN"] != "test-key" {
		t.Errorf("Env token not merged")
	}
	if merged.Env["ANTHROPIC_BASE_URL"] != "https://api.test.com" {
		t.Errorf("Env base URL not merged")
	}
}

func TestMergeEnabledPlugins(t *testing.T) {
	base := DefaultSettings()
	overlay := Settings{
		EnabledPlugins: map[string]bool{
			"gopls-lsp@claude-plugins-official": true,
		},
	}
	merged := Merge(base, overlay)

	if !merged.EnabledPlugins["gopls-lsp@claude-plugins-official"] {
		t.Error("EnabledPlugins not merged")
	}
}

func TestApplyEnv(t *testing.T) {
	s := Settings{
		Env: map[string]string{
			"TEST_GLAW_VAR": "hello",
		},
	}
	s.ApplyEnv()

	if os.Getenv("TEST_GLAW_VAR") != "hello" {
		t.Error("ApplyEnv did not set env var")
	}
	os.Unsetenv("TEST_GLAW_VAR")
}

func TestDeriveFromEnv(t *testing.T) {
	// Clear any lingering env vars from the user's session
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "derived-key")
	t.Setenv("ANTHROPIC_BASE_URL", "https://derived.api")
	t.Setenv("ANTHROPIC_DEFAULT_SONNET_MODEL", "glm-5.1")

	s := DefaultSettings()
	s.deriveFromEnv()

	// ANTHROPIC_DEFAULT_SONNET_MODEL should override the default model
	if s.Model != "glm-5.1" {
		t.Errorf("Model = %q, want %q", s.Model, "glm-5.1")
	}
	// After model override to glm-5.1 (non-prefixed → Anthropic), API key comes from ANTHROPIC_AUTH_TOKEN
	if s.APIKey != "derived-key" {
		t.Errorf("APIKey = %q, want %q", s.APIKey, "derived-key")
	}
	if s.APIBaseURL != "https://derived.api" {
		t.Errorf("APIBaseURL = %q", s.APIBaseURL)
	}
}

func TestDeriveFromEnvOpenRouter(t *testing.T) {
	// Clear model override and other provider keys to test openrouter in isolation
	t.Setenv("ANTHROPIC_DEFAULT_SONNET_MODEL", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "or-test-key")
	t.Setenv("OPENROUTER_BASE_URL", "")

	s := DefaultSettings() // model is openrouter:nvidia/nemotron-3-super-120b-a12b:free
	s.deriveFromEnv()

	if s.APIKey != "or-test-key" {
		t.Errorf("APIKey = %q, want %q", s.APIKey, "or-test-key")
	}
}

func TestDeriveFromEnvAnthropicModel(t *testing.T) {
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "anthropic-key")
	t.Setenv("ANTHROPIC_BASE_URL", "https://custom.api")
	t.Setenv("ANTHROPIC_DEFAULT_SONNET_MODEL", "")

	s := Settings{
		Model:        "claude-sonnet-4-6",
		Permissions:  PermissionSettings{Mode: "workspace_write"},
		MaxTokens:    16384,
		Temperature:  func() *float64 { f := 1.0; return &f }(),
	}
	s.deriveFromEnv()

	if s.APIKey != "anthropic-key" {
		t.Errorf("APIKey = %q, want %q", s.APIKey, "anthropic-key")
	}
	if s.APIBaseURL != "https://custom.api" {
		t.Errorf("APIBaseURL = %q", s.APIBaseURL)
	}
}

func TestLoadAllWithEnvBlock(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ANTHROPIC_DEFAULT_SONNET_MODEL", "")

	// Write settings with env block (matches real ~/.glaw/settings.json)
	globalDir := filepath.Join(home, DefaultGlobalDir)
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}
	settings := Settings{
		Model: "claude-sonnet-4-6", // explicitly set Anthropic model
		Env: map[string]string{
			"ANTHROPIC_AUTH_TOKEN": "test-token-123",
			"ANTHROPIC_BASE_URL":   "https://api.z.ai/api/anthropic",
		},
		EnabledPlugins: map[string]bool{
			"gopls-lsp@claude-plugins-official": true,
		},
	}
	data, _ := json.MarshalIndent(settings, "", "  ")
	if err := os.WriteFile(filepath.Join(globalDir, SettingsFileName), data, 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	loaded, err := LoadAll(workspace)
	if err != nil {
		t.Fatalf("LoadAll error: %v", err)
	}

	// Env vars should be applied
	if os.Getenv("ANTHROPIC_AUTH_TOKEN") != "test-token-123" {
		t.Error("env var not applied")
	}
	// Derived fields populated from env
	if loaded.APIKey != "test-token-123" {
		t.Errorf("APIKey = %q, want derived from env", loaded.APIKey)
	}
	if loaded.APIBaseURL != "https://api.z.ai/api/anthropic" {
		t.Errorf("APIBaseURL = %q", loaded.APIBaseURL)
	}
	// EnabledPlugins preserved
	if !loaded.EnabledPlugins["gopls-lsp@claude-plugins-official"] {
		t.Error("EnabledPlugins not loaded")
	}
}
