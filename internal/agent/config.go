package agent

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SubAgentConfig defines a sub-agent as specified in a markdown file with YAML
// frontmatter. This mirrors Claude Code's sub-agent configuration format:
//
//	---
//	name: my-agent
//	description: MUST BE USED for code reviews
//	tools: Read, Write, Edit, Grep, Bash
//	model: sonnet
//	---
//
//	System prompt instructions go here.
type SubAgentConfig struct {
	// Name is the unique identifier for the sub-agent. If empty, it is
	// inferred from the filename (without the .md extension).
	Name string `json:"name"`

	// Description is an action-oriented description used by the parent agent
	// to decide when to delegate to this sub-agent.
	Description string `json:"description"`

	// Tools is the list of tool names the sub-agent is allowed to use. If
	// empty, the sub-agent inherits ALL tools from the parent.
	// Valid tool names: bash, read_file, write_file, edit_file, glob_search,
	// grep_search, web_fetch, web_search, todo_write, notebook_edit, sleep
	Tools []string `json:"tools,omitempty"`

	// Model specifies which model to use. Options: "sonnet", "opus", "haiku",
	// or "inherit" (use the same model as the parent). Default: "inherit".
	Model string `json:"model,omitempty"`

	// Prompt is the system prompt for the sub-agent, taken from the body of
	// the markdown file (after the YAML frontmatter).
	Prompt string `json:"prompt"`

	// SourcePath is the filesystem path the config was loaded from.
	SourcePath string `json:"source_path,omitempty"`

	// Level indicates where the config was loaded from: "project" or "user".
	Level string `json:"level,omitempty"`
}

// Map from Claude Code tool names to glaw internal tool names.
var claudeToolToGlaw = map[string]string{
	"Read":     "read_file",
	"Write":    "write_file",
	"Edit":     "edit_file",
	"View":     "read_file",
	"Bash":     "bash",
	"Grep":     "grep_search",
	"Glob":     "glob_search",
	"WebFetch": "web_fetch",
	"WebSearch": "web_search",
	"TodoRead":  "todo_write",
	"TodoWrite": "todo_write",
}

// ParseSubAgentConfig parses a markdown file with YAML frontmatter into a
// SubAgentConfig. The file format is:
//
//	---
//	name: agent-name
//	description: What this agent does
//	tools: Read, Write, Edit, Grep, Bash
//	model: sonnet
//	---
//
//	System prompt instructions...
func ParseSubAgentConfig(data []byte, filename string) (*SubAgentConfig, error) {
	config := &SubAgentConfig{}

	// Infer name from filename if not provided
	base := filepath.Base(filename)
	nameFromFilename := strings.TrimSuffix(base, filepath.Ext(base))

	// Split into frontmatter and body
	fm, body, err := splitFrontmatter(data)
	if err != nil {
		return nil, err
	}

	// Parse frontmatter key-value pairs
	for _, line := range fm {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := parseKeyValue(line)
		if !ok {
			continue
		}

		switch strings.ToLower(key) {
		case "name":
			config.Name = strings.TrimSpace(value)
		case "description":
			config.Description = strings.TrimSpace(value)
		case "tools":
			config.Tools = parseToolList(value)
		case "model":
			config.Model = strings.TrimSpace(strings.ToLower(value))
		}
	}

	// Default name from filename
	if config.Name == "" {
		config.Name = nameFromFilename
	}

	// Body is the system prompt
	config.Prompt = strings.TrimSpace(body)

	if config.Name == "" {
		return nil, fmt.Errorf("sub-agent config has no name (filename: %q)", filename)
	}

	return config, nil
}

// splitFrontmatter splits data into YAML frontmatter lines and body.
// It expects the data to start with "---\n" and end the frontmatter with "\n---".
func splitFrontmatter(data []byte) ([]string, string, error) {
	content := string(data)

	// Must start with ---
	if !strings.HasPrefix(content, "---") {
		// No frontmatter, entire content is body
		return nil, content, nil
	}

	// Find the closing ---
	// Skip the opening ---
	rest := content[3:]
	// Skip optional newline after opening ---
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}

	// Find closing ---
	closingIdx := -1
	for i := 0; i < len(rest); i++ {
		if rest[i] == '-' {
			if i+2 < len(rest) && rest[i:i+3] == "---" {
				// Check that --- is at the start of a line or is preceded by newline
				if i == 0 || rest[i-1] == '\n' {
					closingIdx = i
					break
				}
			}
		}
	}

	if closingIdx < 0 {
		return nil, content, fmt.Errorf("unclosed frontmatter: missing closing ---")
	}

	fmContent := rest[:closingIdx]
	// Trim trailing newline from frontmatter
	fmContent = strings.TrimRight(fmContent, "\r\n")

	// Body starts after the closing ---
	body := rest[closingIdx+3:]
	// Trim leading newlines from body
	body = strings.TrimLeft(body, "\r\n")

	fmLines := strings.Split(fmContent, "\n")
	return fmLines, body, nil
}

// parseKeyValue parses a "key: value" line.
func parseKeyValue(line string) (key, value string, ok bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	value = strings.TrimSpace(line[idx+1:])
	return key, value, true
}

// parseToolList parses a comma-separated list of tool names, converting
// Claude Code tool names to glaw internal names.
func parseToolList(s string) []string {
	parts := strings.Split(s, ",")
	var tools []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Try Claude Code tool name mapping first
		if glawName, ok := claudeToolToGlaw[p]; ok {
			tools = append(tools, glawName)
		} else {
			// Use as-is (support glaw-native tool names too)
			tools = append(tools, strings.ToLower(p))
		}
	}
	return tools
}

// ResolvedTools returns the full set of tool names the sub-agent is allowed to
// use. If config.Tools is empty, all tools are returned (inherit from parent).
func (c *SubAgentConfig) ResolvedTools(allTools []string) []string {
	if len(c.Tools) == 0 {
		return allTools
	}
	return c.Tools
}

// ResolvedModel returns the model to use, resolving "inherit" and empty values.
func (c *SubAgentConfig) ResolvedModel(parentModel string) string {
	switch c.Model {
	case "", "inherit":
		return parentModel
	case "sonnet":
		return "claude-sonnet-4-6"
	case "opus":
		return "claude-opus-4"
	case "haiku":
		return "claude-haiku-4"
	default:
		return c.Model
	}
}

// LoadSubAgentsFromDir loads all .md sub-agent configs from a directory.
// Returns configs sorted by name. Files that fail to parse are skipped
// (errors are reported via the errors slice).
func LoadSubAgentsFromDir(dir string, level string) ([]*SubAgentConfig, []error) {
	var configs []*SubAgentConfig
	var errs []error

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil // directory doesn't exist is not an error
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".md" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("reading %s: %w", path, err))
			continue
		}

		config, err := ParseSubAgentConfig(data, entry.Name())
		if err != nil {
			errs = append(errs, fmt.Errorf("parsing %s: %w", path, err))
			continue
		}

		config.SourcePath = path
		config.Level = level
		configs = append(configs, config)
	}

	// Sort by name
	sort.Slice(configs, func(i, j int) bool {
		return configs[i].Name < configs[j].Name
	})

	return configs, errs
}

// LoadAllSubAgents loads sub-agents from both user-level and project-level
// directories. Project-level configs take precedence over user-level configs
// with the same name.
//
// Directories searched:
//   - User-level:   ~/.glaw/agents/
//   - Project-level: {workspaceRoot}/.glaw/agents/
func LoadAllSubAgents(workspaceRoot string) ([]*SubAgentConfig, error) {
	seen := make(map[string]*SubAgentConfig)

	// Load user-level agents
	home, err := os.UserHomeDir()
	if err == nil {
		userDir := filepath.Join(home, ".glaw", "agents")
		userConfigs, _ := LoadSubAgentsFromDir(userDir, "user")
		for _, c := range userConfigs {
			seen[c.Name] = c
		}
	}

	// Load project-level agents (take precedence)
	projectDir := filepath.Join(workspaceRoot, ".glaw", "agents")
	projectConfigs, _ := LoadSubAgentsFromDir(projectDir, "project")
	for _, c := range projectConfigs {
		seen[c.Name] = c
	}

	// Collect and sort
	var result []*SubAgentConfig
	for _, c := range seen {
		result = append(result, c)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

// CreateSubAgentFile creates a new sub-agent markdown file in the specified
// agents directory.
func CreateSubAgentFile(dir string, config *SubAgentConfig) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating agents directory: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.WriteString(fmt.Sprintf("name: %s\n", config.Name))
	buf.WriteString(fmt.Sprintf("description: %s\n", config.Description))
	if len(config.Tools) > 0 {
		buf.WriteString(fmt.Sprintf("tools: %s\n", strings.Join(config.Tools, ", ")))
	}
	if config.Model != "" {
		buf.WriteString(fmt.Sprintf("model: %s\n", config.Model))
	}
	buf.WriteString("---\n\n")
	buf.WriteString(config.Prompt)
	buf.WriteString("\n")

	path := filepath.Join(dir, config.Name+".md")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("writing agent file: %w", err)
	}

	return nil
}

// EnsureAgentsDir creates the .glaw/agents directory if it doesn't exist
// and returns its path.
func EnsureAgentsDir(workspaceRoot string) (string, error) {
	dir := filepath.Join(workspaceRoot, ".glaw", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating agents directory: %w", err)
	}
	return dir, nil
}

// EnsureUserAgentsDir creates the ~/.glaw/agents directory if it doesn't
// exist and returns its path.
func EnsureUserAgentsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	dir := filepath.Join(home, ".glaw", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating user agents directory: %w", err)
	}
	return dir, nil
}
