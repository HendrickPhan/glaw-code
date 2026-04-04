package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// PermissionResult holds the outcome of a permission check.
type PermissionResult struct {
	Allowed       bool
	Message       string
	DenialReason  string
	CacheDecision CacheDecision // whether this was resolved from cache
}

// CacheDecision indicates how the result was determined.
type CacheDecision int

const (
	CacheNone CacheDecision = iota
	CacheHitAllowed
	CacheHitDenied
)

// CacheKey uniquely identifies a permission request.
type CacheKey struct {
	ToolName string
	Input    string // normalized/truncated input for caching
}

// EnhancedPermissionManager handles permission checking with per-tool allow/deny
// lists, workspace-scoped path validation, and session-level caching.
type EnhancedPermissionManager struct {
	mu sync.RWMutex

	Mode          PermissionMode
	WorkspaceRoot string

	// Per-permission-type allow/deny
	Allowed map[Permission]bool
	Denied  map[Permission]bool

	// Per-tool allow/deny lists
	ToolAllowList map[string]bool
	ToolDenyList  map[string]bool

	// Session-level cache: once approved in "prompt" mode, remember for the session
	Cache map[CacheKey]bool
}

// NewEnhancedPermissionManager creates a new permission manager with the given mode and workspace root.
func NewEnhancedPermissionManager(mode PermissionMode, workspaceRoot string) *EnhancedPermissionManager {
	absRoot, _ := filepath.Abs(workspaceRoot)
	return &EnhancedPermissionManager{
		Mode:          mode,
		WorkspaceRoot: absRoot,
		Allowed:       make(map[Permission]bool),
		Denied:        make(map[Permission]bool),
		ToolAllowList: make(map[string]bool),
		ToolDenyList:  make(map[string]bool),
		Cache:         make(map[CacheKey]bool),
	}
}

// NewEnhancedPermissionManagerFromSettings creates a permission manager from config settings.
// allow and deny are lists of tool names from config.PermissionSettings.
func NewEnhancedPermissionManagerFromSettings(mode PermissionMode, workspaceRoot string, allow []string, deny []string) *EnhancedPermissionManager {
	pm := NewEnhancedPermissionManager(mode, workspaceRoot)
	for _, tool := range allow {
		pm.ToolAllowList[tool] = true
	}
	for _, tool := range deny {
		pm.ToolDenyList[tool] = true
	}
	return pm
}

// CheckToolPermission evaluates whether a tool invocation is permitted.
// It checks: deny lists -> allow lists -> mode-based rules -> path validation -> cache for prompt mode.
// When the mode is "prompt" and no cached decision exists, it returns a result with
// Allowed=false and Message describing what the tool wants to do (the caller should
// then prompt the user and call RecordDecision).
func (m *EnhancedPermissionManager) CheckToolPermission(toolName string, requiredPerm Permission, input json.RawMessage) *PermissionResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 1. Check tool-level deny list first (hard deny)
	if m.ToolDenyList[toolName] {
		return &PermissionResult{
			Allowed:      false,
			Message:      fmt.Sprintf("Tool %q is in the deny list", toolName),
			DenialReason: "tool_deny_list",
		}
	}

	// 2. Check tool-level allow list (hard allow)
	if m.ToolAllowList[toolName] {
		return &PermissionResult{
			Allowed:       true,
			Message:       fmt.Sprintf("Tool %q is in the allow list", toolName),
			CacheDecision: CacheNone,
		}
	}

	// 3. Mode-based evaluation
	switch m.Mode {
	case PermAllow, PermDangerFullAccess:
		return m.checkFullAccess(toolName, requiredPerm, input)
	case PermReadOnly:
		return m.checkReadOnly(toolName, requiredPerm, input)
	case PermWorkspaceWrite:
		return m.checkWorkspaceWrite(toolName, requiredPerm, input)
	case PermPrompt:
		return m.checkPrompt(toolName, requiredPerm, input)
	default:
		return &PermissionResult{
			Allowed:      false,
			DenialReason: "unknown_mode",
			Message:      fmt.Sprintf("Unknown permission mode: %q", m.Mode),
		}
	}
}

// RecordDecision records a user's decision for a tool invocation in prompt mode.
// This caches the decision so subsequent identical requests don't need to re-prompt.
func (m *EnhancedPermissionManager) RecordDecision(toolName string, input json.RawMessage, allowed bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := CacheKey{
		ToolName: toolName,
		Input:    normalizeInput(input),
	}
	m.Cache[key] = allowed
}

// IsToolAllowed returns whether a specific tool name is in the allow list.
func (m *EnhancedPermissionManager) IsToolAllowed(toolName string) bool {
	return m.ToolAllowList[toolName]
}

// IsToolDenied returns whether a specific tool name is in the deny list.
func (m *EnhancedPermissionManager) IsToolDenied(toolName string) bool {
	return m.ToolDenyList[toolName]
}

// GetMode returns the current permission mode.
func (m *EnhancedPermissionManager) GetMode() PermissionMode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Mode
}

// SetMode changes the permission mode.
func (m *EnhancedPermissionManager) SetMode(mode PermissionMode) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Mode = mode
}

// --- Mode-specific check implementations ---

func (m *EnhancedPermissionManager) checkFullAccess(toolName string, requiredPerm Permission, input json.RawMessage) *PermissionResult {
	// Full access allows everything, but still validate paths for file tools.
	if isFileWriteTool(toolName) {
		if err := m.validateFilePath(input); err != nil {
			return &PermissionResult{
				Allowed:      false,
				Message:      err.Error(),
				DenialReason: "path_outside_workspace",
			}
		}
	}
	return &PermissionResult{Allowed: true, Message: "full access granted"}
}

func (m *EnhancedPermissionManager) checkReadOnly(toolName string, requiredPerm Permission, input json.RawMessage) *PermissionResult {
	// Read-only only allows read operations.
	switch requiredPerm {
	case PermReadFile, PermNetwork:
		return &PermissionResult{Allowed: true, Message: "read operation allowed"}
	default:
		return &PermissionResult{
			Allowed:      false,
			Message:      fmt.Sprintf("Tool %q requires %q permission but mode is read_only", toolName, requiredPerm),
			DenialReason: "read_only_mode",
		}
	}
}

func (m *EnhancedPermissionManager) checkWorkspaceWrite(toolName string, requiredPerm Permission, input json.RawMessage) *PermissionResult {
	// Workspace write allows file reads and writes within workspace, but not arbitrary command execution.
	switch requiredPerm {
	case PermReadFile, PermWriteFile, PermEditFile:
		if isFileWriteTool(toolName) {
			if err := m.validateFilePath(input); err != nil {
				return &PermissionResult{
					Allowed:      false,
					Message:      err.Error(),
					DenialReason: "path_outside_workspace",
				}
			}
		}
		return &PermissionResult{Allowed: true, Message: "workspace write allowed"}
	case PermExecuteCommand:
		return &PermissionResult{
			Allowed:      false,
			Message:      fmt.Sprintf("Tool %q requires execute permission but mode is workspace_write", toolName),
			DenialReason: "workspace_write_no_exec",
		}
	default:
		return &PermissionResult{Allowed: true, Message: "allowed"}
	}
}

func (m *EnhancedPermissionManager) checkPrompt(toolName string, requiredPerm Permission, input json.RawMessage) *PermissionResult {
	// In prompt mode, check cache first. If not cached, return a result that
	// signals the caller needs to prompt the user.
	key := CacheKey{
		ToolName: toolName,
		Input:    normalizeInput(input),
	}

	if cached, ok := m.Cache[key]; ok {
		if cached {
			return &PermissionResult{
				Allowed:       true,
				Message:       "previously approved this session",
				CacheDecision: CacheHitAllowed,
			}
		}
		return &PermissionResult{
			Allowed:       false,
			Message:       "previously denied this session",
			DenialReason:  "session_cache_deny",
			CacheDecision: CacheHitDenied,
		}
	}

	// Path validation still applies regardless of prompting.
	if isFileWriteTool(toolName) {
		if err := m.validateFilePath(input); err != nil {
			return &PermissionResult{
				Allowed:      false,
				Message:      err.Error(),
				DenialReason: "path_outside_workspace",
			}
		}
	}

	// Return with Allowed=false and a descriptive message so the caller can
	// present a prompt to the user.
	description := m.describeToolAction(toolName, input)
	return &PermissionResult{
		Allowed:      false, // caller must prompt and call RecordDecision
		Message:      description,
		DenialReason: "needs_prompt",
	}
}

// --- Path validation ---

// validateFilePath checks that the target path in a file tool input is within
// the workspace root. It resolves symlinks and cleans the path to prevent
// traversal attacks.
func (m *EnhancedPermissionManager) validateFilePath(input json.RawMessage) error {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		// If we can't parse the path, we can't validate it. Allow it and let
		// the tool itself report the error.
		return nil
	}
	if args.Path == "" {
		return nil
	}

	return ValidatePathWithinWorkspace(args.Path, m.WorkspaceRoot)
}

// ValidatePathWithinWorkspace checks that a path is within the workspace root.
func ValidatePathWithinWorkspace(targetPath, workspaceRoot string) error {
	// Resolve the target path to an absolute path.
	absPath := targetPath
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(workspaceRoot, absPath)
	}

	// Clean the path to remove any ".." components.
	absPath = filepath.Clean(absPath)
	workspaceRoot = filepath.Clean(workspaceRoot)

	// Resolve symlinks consistently for both paths.
	// On macOS, /var is a symlink to /private/var, etc.
	absPath = resolveSymlinks(absPath)
	workspaceRoot = resolveSymlinks(workspaceRoot)

	// Ensure the path is under workspace root.
	if !isPathWithin(absPath, workspaceRoot) {
		// Show a relative hint for clarity.
		rel, err := filepath.Rel(workspaceRoot, absPath)
		hint := absPath
		if err == nil {
			hint = rel
		}
		return fmt.Errorf("path %q is outside workspace root %q", hint, workspaceRoot)
	}

	return nil
}

// resolveSymlinks resolves symlinks in the path. If the full path doesn't exist,
// it walks up to find the longest existing prefix and resolves that, then
// appends the remaining components. This handles macOS /var -> /private/var
// correctly even for paths to files that don't exist yet.
func resolveSymlinks(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved
	}

	// Walk up the path tree to find an existing prefix.
	dir := path
	var remaining []string
	for {
		resolved, err := filepath.EvalSymlinks(dir)
		if err == nil {
			// Found an existing prefix. Append remaining components.
			for i := len(remaining) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, remaining[i])
			}
			return resolved
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root without finding an existing path.
			return path
		}
		remaining = append(remaining, filepath.Base(dir))
		dir = parent
	}
}

// isPathWithin checks whether the given path is within the parent directory.
func isPathWithin(path, parent string) bool {
	// Ensure trailing separator for accurate prefix matching.
	if !strings.HasSuffix(parent, string(os.PathSeparator)) {
		parent += string(os.PathSeparator)
	}
	return strings.HasPrefix(path+string(os.PathSeparator), parent) || path == strings.TrimSuffix(parent, string(os.PathSeparator))
}

// --- Utility helpers ---

// isFileWriteTool returns true for tools that write or edit files.
func isFileWriteTool(name string) bool {
	return name == "write_file" || name == "edit_file"
}

// normalizeInput produces a canonical string from raw JSON for cache key purposes.
func normalizeInput(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	// Compact the JSON to normalize whitespace.
	var buf bytes.Buffer
	if err := json.Compact(&buf, input); err != nil {
		return string(input)
	}
	return buf.String()
}

// describeToolAction produces a human-readable description of what a tool will do.
func (m *EnhancedPermissionManager) describeToolAction(toolName string, input json.RawMessage) string {
	switch toolName {
	case "bash":
		var args struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(input, &args); err == nil {
			cmd := args.Command
			if len(cmd) > 200 {
				cmd = cmd[:200] + "..."
			}
			return fmt.Sprintf("Execute shell command: %s", cmd)
		}
		return "Execute shell command"

	case "write_file":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(input, &args); err == nil {
			return fmt.Sprintf("Write file: %s", args.Path)
		}
		return "Write file"

	case "edit_file":
		var args struct {
			Path      string `json:"path"`
			OldString string `json:"old_string"`
			NewString string `json:"new_string"`
		}
		if err := json.Unmarshal(input, &args); err == nil {
			return fmt.Sprintf("Edit file: %s (replace %d chars with %d chars)",
				args.Path, len(args.OldString), len(args.NewString))
		}
		return "Edit file"

	case "read_file":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(input, &args); err == nil {
			return fmt.Sprintf("Read file: %s", args.Path)
		}
		return "Read file"

	default:
		return fmt.Sprintf("Execute tool: %s", toolName)
	}
}

// OSName returns the current operating system name.
func OSName() string {
	return runtime.GOOS
}
