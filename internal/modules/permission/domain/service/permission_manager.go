package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hieu-glaw/glaw-code/internal/modules/permission/domain/entity"
	"github.com/hieu-glaw/glaw-code/internal/modules/permission/domain/valueobject"
)

// PermissionManager handles permission checking.
type PermissionManager struct {
	Mode          entity.PermissionMode
	WorkspaceRoot string
	Allowed       map[entity.Permission]bool
	PreviousMode  entity.PermissionMode
}

// NewPermissionManager creates a new permission manager.
func NewPermissionManager(mode entity.PermissionMode, workspaceRoot string) *PermissionManager {
	return &PermissionManager{
		Mode:          mode,
		WorkspaceRoot: workspaceRoot,
		Allowed:       make(map[entity.Permission]bool),
	}
}

// IsYolo returns true if yolo mode is active.
func (m *PermissionManager) IsYolo() bool {
	return m.Mode == entity.PermYolo
}

// ToggleYolo enables or disables yolo mode.
func (m *PermissionManager) ToggleYolo() bool {
	if m.Mode == entity.PermYolo {
		if m.PreviousMode != "" {
			m.Mode = m.PreviousMode
		} else {
			m.Mode = entity.PermWorkspaceWrite
		}
		m.PreviousMode = ""
		return false
	}
	m.PreviousMode = m.Mode
	m.Mode = entity.PermYolo
	return true
}

// Check evaluates whether a permission is granted.
func (m *PermissionManager) Check(perm entity.Permission) bool {
	switch m.Mode {
	case entity.PermAllow, entity.PermDangerFullAccess, entity.PermYolo:
		return true
	case entity.PermReadOnly:
		return perm == entity.PermReadFile || perm == entity.PermNetwork
	case entity.PermWorkspaceWrite:
		return perm != entity.PermExecuteCommand
	case entity.PermPrompt:
		return true
	default:
		return false
	}
}

// CheckTool evaluates whether a tool can be used based on required permission.
func (m *PermissionManager) CheckTool(toolName string, requiredPerm entity.PermissionMode) bool {
	switch m.Mode {
	case entity.PermAllow, entity.PermDangerFullAccess, entity.PermYolo:
		return true
	case entity.PermReadOnly:
		return requiredPerm == entity.PermReadOnly || requiredPerm == ""
	case entity.PermWorkspaceWrite:
		return requiredPerm != entity.PermDangerFullAccess
	case entity.PermPrompt:
		return true
	default:
		return false
	}
}

// GetMode returns the current permission mode.
func (m *PermissionManager) GetMode() entity.PermissionMode {
	return m.Mode
}

// GetWorkspaceRoot returns the workspace root.
func (m *PermissionManager) GetWorkspaceRoot() string {
	return m.WorkspaceRoot
}

// EnhancedPermissionManager handles permission checking with per-tool allow/deny
// lists, workspace-scoped path validation, and session-level caching.
type EnhancedPermissionManager struct {
	mu sync.RWMutex

	Mode          entity.PermissionMode
	WorkspaceRoot string

	Allowed map[entity.Permission]bool
	Denied  map[entity.Permission]bool

	ToolAllowList map[string]bool
	ToolDenyList  map[string]bool

	Cache map[valueobject.CacheKey]bool
}

// NewEnhancedPermissionManager creates a new permission manager.
func NewEnhancedPermissionManager(mode entity.PermissionMode, workspaceRoot string) *EnhancedPermissionManager {
	absRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		absRoot = workspaceRoot
	}
	return &EnhancedPermissionManager{
		Mode:          mode,
		WorkspaceRoot: absRoot,
		Allowed:       make(map[entity.Permission]bool),
		Denied:        make(map[entity.Permission]bool),
		ToolAllowList: make(map[string]bool),
		ToolDenyList:  make(map[string]bool),
		Cache:         make(map[valueobject.CacheKey]bool),
	}
}

// NewEnhancedPermissionManagerFromSettings creates a permission manager from config settings.
func NewEnhancedPermissionManagerFromSettings(mode entity.PermissionMode, workspaceRoot string, allow []string, deny []string) *EnhancedPermissionManager {
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
func (m *EnhancedPermissionManager) CheckToolPermission(toolName string, requiredPerm entity.Permission, input json.RawMessage) *entity.PermissionResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.ToolDenyList[toolName] {
		return &entity.PermissionResult{
			Allowed:      false,
			Message:      fmt.Sprintf("Tool %q is in the deny list", toolName),
			DenialReason: "tool_deny_list",
		}
	}

	if m.ToolAllowList[toolName] {
		return &entity.PermissionResult{
			Allowed:       true,
			Message:       fmt.Sprintf("Tool %q is in the allow list", toolName),
			CacheDecision: entity.CacheNone,
		}
	}

	switch m.Mode {
	case entity.PermAllow, entity.PermDangerFullAccess:
		return m.checkFullAccess(toolName, requiredPerm, input)
	case entity.PermReadOnly:
		return m.checkReadOnly(toolName, requiredPerm, input)
	case entity.PermWorkspaceWrite:
		return m.checkWorkspaceWrite(toolName, requiredPerm, input)
	case entity.PermPrompt:
		return m.checkPrompt(toolName, requiredPerm, input)
	default:
		return &entity.PermissionResult{
			Allowed:      false,
			DenialReason: "unknown_mode",
			Message:      fmt.Sprintf("Unknown permission mode: %q", m.Mode),
		}
	}
}

// RecordDecision records a user's decision for a tool invocation in prompt mode.
func (m *EnhancedPermissionManager) RecordDecision(toolName string, input json.RawMessage, allowed bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := valueobject.CacheKey{
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
func (m *EnhancedPermissionManager) GetMode() entity.PermissionMode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Mode
}

// SetMode changes the permission mode.
func (m *EnhancedPermissionManager) SetMode(mode entity.PermissionMode) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Mode = mode
}

func (m *EnhancedPermissionManager) checkFullAccess(toolName string, requiredPerm entity.Permission, input json.RawMessage) *entity.PermissionResult {
	if isFileWriteTool(toolName) {
		if err := ValidatePathWithinWorkspace(input, m.WorkspaceRoot); err != nil {
			return &entity.PermissionResult{
				Allowed:      false,
				Message:      err.Error(),
				DenialReason: "path_outside_workspace",
			}
		}
	}
	return &entity.PermissionResult{Allowed: true, Message: "full access granted"}
}

func (m *EnhancedPermissionManager) checkReadOnly(toolName string, requiredPerm entity.Permission, input json.RawMessage) *entity.PermissionResult {
	switch requiredPerm {
	case entity.PermReadFile, entity.PermNetwork:
		return &entity.PermissionResult{Allowed: true, Message: "read operation allowed"}
	default:
		return &entity.PermissionResult{
			Allowed:      false,
			Message:      fmt.Sprintf("Tool %q requires %q permission but mode is read_only", toolName, requiredPerm),
			DenialReason: "read_only_mode",
		}
	}
}

func (m *EnhancedPermissionManager) checkWorkspaceWrite(toolName string, requiredPerm entity.Permission, input json.RawMessage) *entity.PermissionResult {
	switch requiredPerm {
	case entity.PermReadFile, entity.PermWriteFile, entity.PermEditFile:
		if isFileWriteTool(toolName) {
			if err := ValidatePathWithinWorkspace(input, m.WorkspaceRoot); err != nil {
				return &entity.PermissionResult{
					Allowed:      false,
					Message:      err.Error(),
					DenialReason: "path_outside_workspace",
				}
			}
		}
		return &entity.PermissionResult{Allowed: true, Message: "workspace write allowed"}
	case entity.PermExecuteCommand:
		return &entity.PermissionResult{
			Allowed:      false,
			Message:      fmt.Sprintf("Tool %q requires execute permission but mode is workspace_write", toolName),
			DenialReason: "workspace_write_no_exec",
		}
	default:
		return &entity.PermissionResult{Allowed: true, Message: "allowed"}
	}
}

func (m *EnhancedPermissionManager) checkPrompt(toolName string, requiredPerm entity.Permission, input json.RawMessage) *entity.PermissionResult {
	key := valueobject.CacheKey{
		ToolName: toolName,
		Input:    normalizeInput(input),
	}

	if cached, ok := m.Cache[key]; ok {
		if cached {
			return &entity.PermissionResult{
				Allowed:       true,
				Message:       "previously approved this session",
				CacheDecision: entity.CacheHitAllowed,
			}
		}
		return &entity.PermissionResult{
			Allowed:       false,
			Message:       "previously denied this session",
			DenialReason:  "session_cache_deny",
			CacheDecision: entity.CacheHitDenied,
		}
	}

	if isFileWriteTool(toolName) {
		if err := ValidatePathWithinWorkspace(input, m.WorkspaceRoot); err != nil {
			return &entity.PermissionResult{
				Allowed:      false,
				Message:      err.Error(),
				DenialReason: "path_outside_workspace",
			}
		}
	}

	description := describeToolAction(toolName, input)
	return &entity.PermissionResult{
		Allowed:      false,
		Message:      description,
		DenialReason: "needs_prompt",
	}
}

// ValidatePathWithinWorkspace checks that the target path in tool input is within workspace.
func ValidatePathWithinWorkspace(input json.RawMessage, workspaceRoot string) error {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil
	}
	if args.Path == "" {
		return nil
	}
	return ValidatePathAbsWithinWorkspace(args.Path, workspaceRoot)
}

// ValidatePathAbsWithinWorkspace checks that a path is within the workspace root.
func ValidatePathAbsWithinWorkspace(targetPath, workspaceRoot string) error {
	absPath := targetPath
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(workspaceRoot, absPath)
	}

	absPath = filepath.Clean(absPath)
	workspaceRoot = filepath.Clean(workspaceRoot)

	absPath = resolveSymlinks(absPath)
	workspaceRoot = resolveSymlinks(workspaceRoot)

	if !isPathWithin(absPath, workspaceRoot) {
		rel, err := filepath.Rel(workspaceRoot, absPath)
		hint := absPath
		if err == nil {
			hint = rel
		}
		return fmt.Errorf("path %q is outside workspace root %q", hint, workspaceRoot)
	}

	return nil
}

func resolveSymlinks(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved
	}

	dir := path
	var remaining []string
	for {
		resolved, err := filepath.EvalSymlinks(dir)
		if err == nil {
			for i := len(remaining) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, remaining[i])
			}
			return resolved
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return path
		}
		remaining = append(remaining, filepath.Base(dir))
		dir = parent
	}
}

func isPathWithin(path, parent string) bool {
	if !strings.HasSuffix(parent, string(os.PathSeparator)) {
		parent += string(os.PathSeparator)
	}
	return strings.HasPrefix(path+string(os.PathSeparator), parent) || path == strings.TrimSuffix(parent, string(os.PathSeparator))
}

func isFileWriteTool(name string) bool {
	return name == "write_file" || name == "edit_file"
}

func normalizeInput(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, input); err != nil {
		return string(input)
	}
	return buf.String()
}

func describeToolAction(toolName string, input json.RawMessage) string {
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
