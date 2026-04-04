package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"time"
)

// HookEvent identifies a point in the lifecycle where hooks run.
type HookEvent string

const (
	HookPreTool       HookEvent = "pre_tool"
	HookPostTool      HookEvent = "post_tool"
	HookPreMessage    HookEvent = "pre_message"
	HookPostMessage   HookEvent = "post_message"
	HookSessionStart  HookEvent = "session_start"
	HookSessionEnd    HookEvent = "session_end"
)

// HookConfig defines how a hook is executed.
type HookConfig struct {
	Command string            `json:"command"`
	Timeout int               `json:"timeout,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// ToolConfig defines a tool provided by a plugin.
type ToolConfig struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
	Command     string          `json:"command"`
}

// Manifest describes a plugin.
type Manifest struct {
	Name        string               `json:"name"`
	Version     string               `json:"version"`
	Description string               `json:"description"`
	Hooks       map[string]HookConfig `json:"hooks,omitempty"`
	Tools       []ToolConfig         `json:"tools,omitempty"`
	Permissions []string             `json:"permissions,omitempty"`
}

// Plugin holds a loaded plugin and its metadata.
type Plugin struct {
	Manifest Manifest
	Path     string
	Enabled  bool
}

// ToolDefinition is a tool definition returned by the manager.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
	Command     string          `json:"command"`
}

// Manager manages loaded plugins.
type Manager struct {
	plugins       map[string]*Plugin
	workspaceRoot string
}

// NewManager creates a new plugin manager.
func NewManager(workspaceRoot string) *Manager {
	return &Manager{
		plugins:       make(map[string]*Plugin),
		workspaceRoot: workspaceRoot,
	}
}

// LoadPlugin loads a plugin from a manifest file path.
func (m *Manager) LoadPlugin(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading plugin manifest: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("parsing plugin manifest: %w", err)
	}

	if err := ValidateManifest(manifest); err != nil {
		return err
	}

	if _, exists := m.plugins[manifest.Name]; exists {
		return fmt.Errorf("plugin %q already loaded", manifest.Name)
	}

	m.plugins[manifest.Name] = &Plugin{
		Manifest: manifest,
		Path:     path,
		Enabled:  true,
	}
	return nil
}

// UnloadPlugin removes a plugin by name.
func (m *Manager) UnloadPlugin(name string) error {
	if _, ok := m.plugins[name]; !ok {
		return fmt.Errorf("plugin %q not found", name)
	}
	delete(m.plugins, name)
	return nil
}

// GetPlugin retrieves a plugin by name.
func (m *Manager) GetPlugin(name string) (*Plugin, bool) {
	p, ok := m.plugins[name]
	return p, ok
}

// ListPlugins returns all loaded plugins.
func (m *Manager) ListPlugins() []Plugin {
	result := make([]Plugin, 0, len(m.plugins))
	for _, p := range m.plugins {
		result = append(result, *p)
	}
	return result
}

// RunHook executes all hooks registered for the given event.
func (m *Manager) RunHook(ctx context.Context, event HookEvent, payload interface{}) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling hook payload: %w", err)
	}

	for _, plugin := range m.plugins {
		if !plugin.Enabled {
			continue
		}
		hookCfg, ok := plugin.Manifest.Hooks[string(event)]
		if !ok {
			continue
		}

		timeout := time.Duration(hookCfg.Timeout) * time.Second
		if timeout == 0 {
			timeout = 30 * time.Second
		}

		hookCtx, cancel := context.WithTimeout(ctx, timeout)
		cmd := exec.CommandContext(hookCtx, "bash", "-c", hookCfg.Command)

		// Set environment variables
		if len(hookCfg.Env) > 0 {
			cmd.Env = os.Environ()
			for k, v := range hookCfg.Env {
				cmd.Env = append(cmd.Env, k+"="+v)
			}
		}

		cmd.Stdin = bytes.NewReader(payloadJSON)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			cancel()
			return fmt.Errorf("hook %s/%s failed: %v: %s", plugin.Manifest.Name, event, err, stderr.String())
		}
		cancel()
	}

	return nil
}

// GetToolDefinitions collects tool definitions from all enabled plugins.
func (m *Manager) GetToolDefinitions() []ToolDefinition {
	var defs []ToolDefinition
	for _, p := range m.plugins {
		if !p.Enabled {
			continue
		}
		for _, tc := range p.Manifest.Tools {
			defs = append(defs, ToolDefinition(tc))
		}
	}
	return defs
}

// ValidateManifest checks that a manifest has required fields.
func ValidateManifest(m Manifest) error {
	if m.Name == "" {
		return fmt.Errorf("plugin name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("plugin version is required")
	}
	// Basic semver-like check: must contain at least one digit
	if !regexp.MustCompile(`\d`).MatchString(m.Version) {
		return fmt.Errorf("plugin version must contain a number")
	}
	return nil
}
