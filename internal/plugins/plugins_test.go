package plugins

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPlugin(t *testing.T) {
	dir := t.TempDir()
	manifest := Manifest{
		Name:        "test-plugin",
		Version:     "1.0.0",
		Description: "A test plugin",
		Hooks: map[string]HookConfig{
			"pre_tool": {Command: "echo pre", Timeout: 5},
		},
		Tools: []ToolConfig{
			{Name: "test_tool", Description: "A test tool", Command: "echo tool"},
		},
	}
	writeManifest(t, dir, "plugin.json", manifest)

	m := NewManager(dir)
	if err := m.LoadPlugin(filepath.Join(dir, "plugin.json")); err != nil {
		t.Fatalf("LoadPlugin error: %v", err)
	}

	p, ok := m.GetPlugin("test-plugin")
	if !ok {
		t.Fatal("plugin not found after loading")
	}
	if p.Manifest.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", p.Manifest.Version, "1.0.0")
	}
	if !p.Enabled {
		t.Error("plugin should be enabled by default")
	}
}

func TestLoadPluginMissingName(t *testing.T) {
	dir := t.TempDir()
	manifest := Manifest{Version: "1.0.0"}
	writeManifest(t, dir, "bad.json", manifest)

	m := NewManager(dir)
	err := m.LoadPlugin(filepath.Join(dir, "bad.json"))
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestLoadPluginMissingVersion(t *testing.T) {
	dir := t.TempDir()
	manifest := Manifest{Name: "test"}
	writeManifest(t, dir, "bad.json", manifest)

	m := NewManager(dir)
	err := m.LoadPlugin(filepath.Join(dir, "bad.json"))
	if err == nil {
		t.Fatal("expected error for missing version")
	}
}

func TestLoadPluginDuplicate(t *testing.T) {
	dir := t.TempDir()
	manifest := Manifest{Name: "dup", Version: "1.0.0"}
	writeManifest(t, dir, "a.json", manifest)
	writeManifest(t, dir, "b.json", manifest)

	m := NewManager(dir)
	if err := m.LoadPlugin(filepath.Join(dir, "a.json")); err != nil {
		t.Fatal(err)
	}
	err := m.LoadPlugin(filepath.Join(dir, "b.json"))
	if err == nil {
		t.Fatal("expected error for duplicate plugin")
	}
}

func TestUnloadPlugin(t *testing.T) {
	dir := t.TempDir()
	manifest := Manifest{Name: "removeme", Version: "1.0.0"}
	writeManifest(t, dir, "p.json", manifest)

	m := NewManager(dir)
	if err := m.LoadPlugin(filepath.Join(dir, "p.json")); err != nil {
		t.Fatalf("LoadPlugin error: %v", err)
	}

	if err := m.UnloadPlugin("removeme"); err != nil {
		t.Fatalf("UnloadPlugin error: %v", err)
	}
	if _, ok := m.GetPlugin("removeme"); ok {
		t.Error("plugin should be removed")
	}
}

func TestUnloadPluginNotFound(t *testing.T) {
	m := NewManager(t.TempDir())
	err := m.UnloadPlugin("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent plugin")
	}
}

func TestListPlugins(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a", "b", "c"} {
		writeManifest(t, dir, name+".json", Manifest{Name: name, Version: "1.0.0"})
	}

	m := NewManager(dir)
	for _, name := range []string{"a", "b", "c"} {
		if err := m.LoadPlugin(filepath.Join(dir, name+".json")); err != nil {
			t.Fatalf("LoadPlugin(%s) error: %v", name, err)
		}
	}

	list := m.ListPlugins()
	if len(list) != 3 {
		t.Fatalf("expected 3 plugins, got %d", len(list))
	}
}

func TestListPluginsEmpty(t *testing.T) {
	m := NewManager(t.TempDir())
	list := m.ListPlugins()
	if len(list) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(list))
	}
}

func TestValidateManifest(t *testing.T) {
	tests := []struct {
		name    string
		manifest Manifest
		wantErr bool
	}{
		{"valid", Manifest{Name: "test", Version: "1.0.0"}, false},
		{"empty name", Manifest{Name: "", Version: "1.0.0"}, true},
		{"empty version", Manifest{Name: "test", Version: ""}, true},
		{"version no number", Manifest{Name: "test", Version: "abc"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateManifest(tt.manifest)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateManifest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRunHookNoHooks(t *testing.T) {
	m := NewManager(t.TempDir())
	err := m.RunHook(context.Background(), HookPreTool, map[string]string{"tool": "bash"})
	if err != nil {
		t.Errorf("RunHook with no plugins should not error: %v", err)
	}
}

func TestRunHookWithCommand(t *testing.T) {
	dir := t.TempDir()
	manifest := Manifest{
		Name:    "hook-test",
		Version: "1.0.0",
		Hooks: map[string]HookConfig{
			"pre_tool": {Command: "cat", Timeout: 5},
		},
	}
	writeManifest(t, dir, "p.json", manifest)

	m := NewManager(dir)
	if err := m.LoadPlugin(filepath.Join(dir, "p.json")); err != nil {
		t.Fatalf("LoadPlugin error: %v", err)
	}

	payload := map[string]string{"tool": "bash"}
	err := m.RunHook(context.Background(), HookPreTool, payload)
	if err != nil {
		t.Errorf("RunHook error: %v", err)
	}
}

func TestGetToolDefinitions(t *testing.T) {
	dir := t.TempDir()
	manifest := Manifest{
		Name:    "tool-plugin",
		Version: "1.0.0",
		Tools: []ToolConfig{
			{Name: "custom_tool", Description: "Custom tool", InputSchema: json.RawMessage(`{"type":"object"}`), Command: "echo"},
		},
	}
	writeManifest(t, dir, "p.json", manifest)

	m := NewManager(dir)
	if err := m.LoadPlugin(filepath.Join(dir, "p.json")); err != nil {
		t.Fatalf("LoadPlugin error: %v", err)
	}

	defs := m.GetToolDefinitions()
	if len(defs) != 1 {
		t.Fatalf("expected 1 tool def, got %d", len(defs))
	}
	if defs[0].Name != "custom_tool" {
		t.Errorf("Name = %q, want %q", defs[0].Name, "custom_tool")
	}
}

func TestGetToolDefinitionsSkipsDisabled(t *testing.T) {
	dir := t.TempDir()
	manifest := Manifest{
		Name:    "disabled-plugin",
		Version: "1.0.0",
		Tools: []ToolConfig{
			{Name: "t", Description: "T", Command: "echo"},
		},
	}
	writeManifest(t, dir, "p.json", manifest)

	m := NewManager(dir)
	if err := m.LoadPlugin(filepath.Join(dir, "p.json")); err != nil {
		t.Fatalf("LoadPlugin error: %v", err)
	}

	// Disable the plugin
	p, _ := m.GetPlugin("disabled-plugin")
	p.Enabled = false

	defs := m.GetToolDefinitions()
	if len(defs) != 0 {
		t.Errorf("expected 0 tool defs from disabled plugin, got %d", len(defs))
	}
}

func writeManifest(t *testing.T, dir, filename string, m Manifest) {
	t.Helper()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), data, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}
