package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultGlobalDir is the default global config directory name.
const DefaultGlobalDir = ".glaw"

// ProjectDir is the project-level config directory name.
const ProjectDir = ".glaw"

// SettingsFileName is the name of the settings file.
const SettingsFileName = "settings.json"

// PermissionSettings holds permission-related configuration.
type PermissionSettings struct {
	Mode  string   `json:"mode,omitempty"`
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

// PluginSettings holds plugin-related configuration.
type PluginSettings struct {
	Enabled  []string `json:"enabled,omitempty"`
	Disabled []string `json:"disabled,omitempty"`
}

// MCPServerConfig represents a single MCP server configuration.
type MCPServerConfig struct {
	Transport string            `json:"transport"` // "stdio" | "sse" | "http"
	Command  string            `json:"command,omitempty"`
	Args      []string            `json:"args,omitempty"`
	URL       string            `json:"url,omitempty"`
	Headers  map[string]string   `json:"headers,omitempty"`
	Env       map[string]string   `json:"env,omitempty"`
}

// Settings represents the full user configuration file.
type Settings struct {
	Model          string                      `json:"model,omitempty"`
	Permissions     PermissionSettings          `json:"permissions"`
	MaxTokens       int                         `json:"maxTokens,omitempty"`
	Temperature     *float64                    `json:"temperature,omitempty"`
	APIKey          string                     `json:"apiKey,omitempty"`
	APIBaseURL      string                     `json:"apiBaseUrl,omitempty"`
	SystemPrompt    string                     `json:"systemPrompt,omitempty"`
	Plugins         PluginSettings                `json:"plugins"`
	Env             map[string]string             `json:"env,omitempty"`
	EnabledPlugins  map[string]bool               `json:"enabledPlugins,omitempty"`
	MCPServers      map[string]*MCPServerConfig `json:"mcpServers,omitempty"`
}

// DefaultSettings returns the built-in defaults.
func DefaultSettings() Settings {
	t := 1.0
	return Settings{
		Model: "claude-sonnet-4-6",
		Permissions: PermissionSettings{
			Mode: "workspace_write",
		},
		MaxTokens:   16384,
		Temperature: &t,
	}
}

// GlobalConfigDir returns the path to the global config directory (~/.glaw).
func GlobalConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, DefaultGlobalDir), nil
}

// GlobalSettingsPath returns the path to the global settings file.
func GlobalSettingsPath() (string, error) {
	dir, err := GlobalConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, SettingsFileName), nil
}

// ProjectSettingsPath returns the path to the project-level settings file.
func ProjectSettingsPath(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, ProjectDir, SettingsFileName)
}

// LoadFromFile reads a settings file from disk.
// Returns nil settings without error if the file does not exist.
func LoadFromFile(path string) (*Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading settings %s: %w", path, err)
	}

	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing settings %s: %w", path, err)
	}
	return &s, nil
}

// Merge overlays the given settings on top of the base.
// Non-zero values in overlay take precedence.
func Merge(base, overlay Settings) Settings {
	result := base

	if overlay.Model != "" {
		result.Model = overlay.Model
	}
	if overlay.Permissions.Mode != "" {
		result.Permissions.Mode = overlay.Permissions.Mode
	}
	if len(overlay.Permissions.Allow) > 0 {
		result.Permissions.Allow = overlay.Permissions.Allow
	}
	if len(overlay.Permissions.Deny) > 0 {
		result.Permissions.Deny = overlay.Permissions.Deny
	}
	if overlay.MaxTokens != 0 {
		result.MaxTokens = overlay.MaxTokens
	}
	if overlay.Temperature != nil {
		result.Temperature = overlay.Temperature
	}
	if overlay.APIKey != "" {
		result.APIKey = overlay.APIKey
	}
	if overlay.APIBaseURL != "" {
		result.APIBaseURL = overlay.APIBaseURL
	}
	if overlay.SystemPrompt != "" {
		result.SystemPrompt = overlay.SystemPrompt
	}
	if len(overlay.Plugins.Enabled) > 0 {
		result.Plugins.Enabled = overlay.Plugins.Enabled
	}
	if len(overlay.Plugins.Disabled) > 0 {
		result.Plugins.Disabled = overlay.Plugins.Disabled
	}
	if len(overlay.Env) > 0 {
		if result.Env == nil {
			result.Env = make(map[string]string)
		}
		for k, v := range overlay.Env {
			result.Env[k] = v
		}
	}
	if len(overlay.EnabledPlugins) > 0 {
		if result.EnabledPlugins == nil {
			result.EnabledPlugins = make(map[string]bool)
		}
		for k, v := range overlay.EnabledPlugins {
			result.EnabledPlugins[k] = v
		}
	}
	if len(overlay.MCPServers) > 0 {
		if result.MCPServers == nil {
			result.MCPServers = make(map[string]*MCPServerConfig)
		}
		for k, v := range overlay.MCPServers {
			result.MCPServers[k] = v
		}
	}

	return result
}

// ApplyEnv sets all Env entries as process environment variables.
// This allows the settings.json "env" block to configure API keys, base URLs, etc.
func (s *Settings) ApplyEnv() {
	for k, v := range s.Env {
		os.Setenv(k, v)
	}
}

// LoadAll loads settings with proper layering:
// defaults -> global (~/.glaw/settings.json) -> project (.glaw/settings.json)
// After loading, env vars from settings are applied and derived config fields are populated.
func LoadAll(workspaceRoot string) (Settings, error) {
	result := DefaultSettings()

	// Layer 1: global config
	globalPath, err := GlobalSettingsPath()
	if err != nil {
		return result, fmt.Errorf("resolving global config path: %w", err)
	}
	global, err := LoadFromFile(globalPath)
	if err != nil {
		return result, fmt.Errorf("loading global settings: %w", err)
	}
	if global != nil {
		result = Merge(result, *global)
	}

	// Layer 2: project config
	projectPath := ProjectSettingsPath(workspaceRoot)
	project, err := LoadFromFile(projectPath)
	if err != nil {
		return result, fmt.Errorf("loading project settings: %w", err)
	}
	if project != nil {
		result = Merge(result, *project)
	}

	// Layer 3: MCP servers from ~/.claude.json (Claude Code config)
	claudeMCP := LoadClaudeMCP()
	if len(claudeMCP) > 0 {
		result = Merge(result, Settings{MCPServers: claudeMCP})
	}

	// Apply env vars from settings to the process
	result.ApplyEnv()

	// Derive API config from env vars if not explicitly set in settings
	result.deriveFromEnv()

	return result, nil
}

// deriveFromEnv fills in API config from environment variables set by the env block.
func (s *Settings) deriveFromEnv() {
	if s.APIKey == "" {
		if v := os.Getenv("ANTHROPIC_AUTH_TOKEN"); v != "" {
			s.APIKey = v
		} else if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
			s.APIKey = v
		}
	}
	if s.APIBaseURL == "" {
		if v := os.Getenv("ANTHROPIC_BASE_URL"); v != "" {
			s.APIBaseURL = v
		}
	}
	if s.Model == "" || s.Model == "claude-sonnet-4-6" {
		if v := os.Getenv("ANTHROPIC_DEFAULT_SONNET_MODEL"); v != "" {
			s.Model = v
		}
	}
}

// Save writes settings to the given path, creating parent directories as needed.
func Save(path string, s Settings) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling settings: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing settings: %w", err)
	}
	return nil
}

// SaveGlobal writes settings to the global config path.
func SaveGlobal(s Settings) error {
	path, err := GlobalSettingsPath()
	if err != nil {
		return err
	}
	return Save(path, s)
}

// SaveProject writes settings to the project config path.
func SaveProject(workspaceRoot string, s Settings) error {
	return Save(ProjectSettingsPath(workspaceRoot), s)
}

// EnsureGlobalDir creates the global config directory if it doesn't exist.
func EnsureGlobalDir() (string, error) {
	dir, err := GlobalConfigDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating global config dir: %w", err)
	}
	return dir, nil
}

// LoadClaudeMCP reads MCP server configurations from ~/.claude.json.
// This file is used by Claude Code and has the format:
//
//	{"mcpServers": {"name": {"type": "http", "url": "...", "headers": {...}}}}
func LoadClaudeMCP() map[string]*MCPServerConfig {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	path := filepath.Join(home, ".claude.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	// Parse only the mcpServers field; ignore the rest of the file.
	var raw struct {
		MCPServers map[string]struct {
			Type    string            `json:"type"`
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
			Env     map[string]string `json:"env"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	if len(raw.MCPServers) == 0 {
		return nil
	}

	result := make(map[string]*MCPServerConfig, len(raw.MCPServers))
	for name, srv := range raw.MCPServers {
		transport := srv.Type
		if transport == "" {
			transport = "http"
		}
		result[name] = &MCPServerConfig{
			Transport: transport,
			Command:   srv.Command,
			Args:      srv.Args,
			URL:       srv.URL,
			Headers:   srv.Headers,
			Env:       srv.Env,
		}
	}
	return result
}
