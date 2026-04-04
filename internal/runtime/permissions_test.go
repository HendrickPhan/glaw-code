package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewEnhancedPermissionManager(t *testing.T) {
	pm := NewEnhancedPermissionManager(PermWorkspaceWrite, "/tmp/test")
	if pm.Mode != PermWorkspaceWrite {
		t.Errorf("Mode = %q, want %q", pm.Mode, PermWorkspaceWrite)
	}
	if pm.WorkspaceRoot == "" {
		t.Error("WorkspaceRoot should not be empty")
	}
	if pm.ToolAllowList == nil || pm.ToolDenyList == nil || pm.Cache == nil {
		t.Error("maps should be initialized")
	}
}

func TestNewEnhancedPermissionManagerFromSettings(t *testing.T) {
	pm := NewEnhancedPermissionManagerFromSettings(
		PermPrompt, "/tmp/test",
		[]string{"read_file", "bash"},
		[]string{"write_file"},
	)
	if !pm.ToolAllowList["read_file"] {
		t.Error("read_file should be in allow list")
	}
	if !pm.ToolAllowList["bash"] {
		t.Error("bash should be in allow list")
	}
	if !pm.ToolDenyList["write_file"] {
		t.Error("write_file should be in deny list")
	}
}

func TestToolDenyListOverridesMode(t *testing.T) {
	pm := NewEnhancedPermissionManager(PermDangerFullAccess, "/tmp")
	pm.ToolDenyList["bash"] = true

	result := pm.CheckToolPermission("bash", PermExecuteCommand, json.RawMessage(`{"command":"ls"}`))
	if result.Allowed {
		t.Error("denied tool should not be allowed even in full access mode")
	}
	if result.DenialReason != "tool_deny_list" {
		t.Errorf("DenialReason = %q, want %q", result.DenialReason, "tool_deny_list")
	}
}

func TestToolAllowListOverridesReadOnly(t *testing.T) {
	pm := NewEnhancedPermissionManager(PermReadOnly, "/tmp")
	pm.ToolAllowList["bash"] = true

	result := pm.CheckToolPermission("bash", PermExecuteCommand, json.RawMessage(`{"command":"ls"}`))
	if !result.Allowed {
		t.Error("allowed tool should pass even in read_only mode")
	}
}

func TestReadOnlyMode(t *testing.T) {
	pm := NewEnhancedPermissionManager(PermReadOnly, "/tmp")

	// Read should be allowed.
	result := pm.CheckToolPermission("read_file", PermReadFile, json.RawMessage(`{"path":"/tmp/test.txt"}`))
	if !result.Allowed {
		t.Error("read_file should be allowed in read_only mode")
	}

	// Write should be denied.
	result = pm.CheckToolPermission("write_file", PermWriteFile, json.RawMessage(`{"path":"/tmp/test.txt","content":"hello"}`))
	if result.Allowed {
		t.Error("write_file should be denied in read_only mode")
	}

	// Bash should be denied.
	result = pm.CheckToolPermission("bash", PermExecuteCommand, json.RawMessage(`{"command":"ls"}`))
	if result.Allowed {
		t.Error("bash should be denied in read_only mode")
	}
}

func TestWorkspaceWriteMode(t *testing.T) {
	tmpDir := t.TempDir()
	pm := NewEnhancedPermissionManager(PermWorkspaceWrite, tmpDir)

	// File write within workspace should be allowed.
	input := json.RawMessage(`{"path":"test.txt","content":"hello"}`)
	result := pm.CheckToolPermission("write_file", PermWriteFile, input)
	if !result.Allowed {
		t.Error("write_file within workspace should be allowed")
	}

	// Bash should be denied.
	result = pm.CheckToolPermission("bash", PermExecuteCommand, json.RawMessage(`{"command":"ls"}`))
	if result.Allowed {
		t.Error("bash should be denied in workspace_write mode")
	}
}

func TestWorkspaceWriteModePathOutsideWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	pm := NewEnhancedPermissionManager(PermWorkspaceWrite, tmpDir)

	// Writing outside workspace should be denied.
	input := json.RawMessage(`{"path":"/etc/passwd","content":"hello"}`)
	result := pm.CheckToolPermission("write_file", PermWriteFile, input)
	if result.Allowed {
		t.Error("write_file outside workspace should be denied")
	}
	if result.DenialReason != "path_outside_workspace" {
		t.Errorf("DenialReason = %q, want %q", result.DenialReason, "path_outside_workspace")
	}
}

func TestFullAccessMode(t *testing.T) {
	tmpDir := t.TempDir()
	pm := NewEnhancedPermissionManager(PermDangerFullAccess, tmpDir)

	// Bash should be allowed.
	result := pm.CheckToolPermission("bash", PermExecuteCommand, json.RawMessage(`{"command":"ls"}`))
	if !result.Allowed {
		t.Error("bash should be allowed in full access mode")
	}

	// File write within workspace should be allowed.
	input := json.RawMessage(`{"path":"test.txt","content":"hello"}`)
	result = pm.CheckToolPermission("write_file", PermWriteFile, input)
	if !result.Allowed {
		t.Error("write_file within workspace should be allowed in full access")
	}

	// Writing outside workspace should still be blocked by path validation.
	input = json.RawMessage(`{"path":"/etc/something","content":"hello"}`)
	result = pm.CheckToolPermission("write_file", PermWriteFile, input)
	if result.Allowed {
		t.Error("write_file outside workspace should be denied even in full access")
	}
}

func TestPromptModeCacheHit(t *testing.T) {
	pm := NewEnhancedPermissionManager(PermPrompt, "/tmp")

	input := json.RawMessage(`{"command":"ls -la"}`)

	// Record a positive decision.
	pm.RecordDecision("bash", input, true)

	result := pm.CheckToolPermission("bash", PermExecuteCommand, input)
	if !result.Allowed {
		t.Error("cached approval should be returned")
	}
	if result.CacheDecision != CacheHitAllowed {
		t.Errorf("CacheDecision = %d, want %d", result.CacheDecision, CacheHitAllowed)
	}
}

func TestPromptModeCacheDenyHit(t *testing.T) {
	pm := NewEnhancedPermissionManager(PermPrompt, "/tmp")

	input := json.RawMessage(`{"command":"rm -rf /"}`)

	// Record a negative decision.
	pm.RecordDecision("bash", input, false)

	result := pm.CheckToolPermission("bash", PermExecuteCommand, input)
	if result.Allowed {
		t.Error("cached denial should be returned")
	}
	if result.CacheDecision != CacheHitDenied {
		t.Errorf("CacheDecision = %d, want %d", result.CacheDecision, CacheHitDenied)
	}
}

func TestPromptModeNeedsPrompt(t *testing.T) {
	pm := NewEnhancedPermissionManager(PermPrompt, "/tmp")

	input := json.RawMessage(`{"command":"ls"}`)
	result := pm.CheckToolPermission("bash", PermExecuteCommand, input)

	if result.Allowed {
		t.Error("uncached prompt should not auto-allow")
	}
	if result.DenialReason != "needs_prompt" {
		t.Errorf("DenialReason = %q, want %q", result.DenialReason, "needs_prompt")
	}
}

func TestPromptModePathValidationBlocks(t *testing.T) {
	tmpDir := t.TempDir()
	pm := NewEnhancedPermissionManager(PermPrompt, tmpDir)

	// Even in prompt mode, path validation should block writes outside workspace.
	input := json.RawMessage(`{"path":"/etc/evil","content":"hacked"}`)
	result := pm.CheckToolPermission("write_file", PermWriteFile, input)

	if result.Allowed {
		t.Error("path outside workspace should be blocked even in prompt mode")
	}
	if result.DenialReason != "path_outside_workspace" {
		t.Errorf("DenialReason = %q, want %q", result.DenialReason, "path_outside_workspace")
	}
}

func TestRecordDecisionAndCache(t *testing.T) {
	pm := NewEnhancedPermissionManager(PermPrompt, "/tmp")

	input1 := json.RawMessage(`{"command":"ls"}`)
	input2 := json.RawMessage(`{"command":"pwd"}`)

	pm.RecordDecision("bash", input1, true)
	pm.RecordDecision("bash", input2, false)

	r1 := pm.CheckToolPermission("bash", PermExecuteCommand, input1)
	r2 := pm.CheckToolPermission("bash", PermExecuteCommand, input2)

	if !r1.Allowed {
		t.Error("input1 should be cached as allowed")
	}
	if r2.Allowed {
		t.Error("input2 should be cached as denied")
	}
}

// --- Path validation tests ---

func TestValidatePathWithinWorkspace(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"relative within", "foo.txt", false},
		{"subdirectory", "sub/bar.txt", false},
		{"absolute within", filepath.Join(tmpDir, "test.txt"), false},
		{"traversal escape", "../../../etc/passwd", true},
		{"absolute outside", "/etc/passwd", true},
		{"empty path", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePathWithinWorkspace(tt.path, tmpDir)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePathWithinWorkspace(%q) = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

// --- describeToolAction tests ---

func TestDescribeToolAction(t *testing.T) {
	pm := NewEnhancedPermissionManager(PermPrompt, "/tmp")

	tests := []struct {
		name    string
		tool    string
		input   string
		contains string
	}{
		{"bash", "bash", `{"command":"ls -la"}`, "Execute shell command"},
		{"write_file", "write_file", `{"path":"foo.txt","content":"bar"}`, "Write file"},
		{"edit_file", "edit_file", `{"path":"foo.txt","old_string":"a","new_string":"b"}`, "Edit file"},
		{"read_file", "read_file", `{"path":"foo.txt"}`, "Read file"},
		{"unknown", "custom_tool", `{"key":"value"}`, "Execute tool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc := pm.describeToolAction(tt.tool, json.RawMessage(tt.input))
			if !contains(desc, tt.contains) {
				t.Errorf("describeToolAction(%q) = %q, want to contain %q", tt.tool, desc, tt.contains)
			}
		})
	}
}

// --- normalizeInput tests ---

func TestNormalizeInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"whitespace", `  {"a":1}  `, `{"a":1}`},
		{"pretty", "{\n  \"a\": 1\n}", `{"a":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeInput(json.RawMessage(tt.input))
			if got != tt.want {
				t.Errorf("normalizeInput() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsFileWriteTool(t *testing.T) {
	if !isFileWriteTool("write_file") {
		t.Error("write_file should be a file write tool")
	}
	if !isFileWriteTool("edit_file") {
		t.Error("edit_file should be a file write tool")
	}
	if isFileWriteTool("bash") {
		t.Error("bash should not be a file write tool")
	}
	if isFileWriteTool("read_file") {
		t.Error("read_file should not be a file write tool")
	}
}

func TestIsPathWithin(t *testing.T) {
	tests := []struct {
		path   string
		parent string
		want   bool
	}{
		{"/home/user/project/file.txt", "/home/user/project", true},
		{"/home/user/project/sub/file.txt", "/home/user/project", true},
		{"/home/user/project", "/home/user/project", true},
		{"/home/user/other/file.txt", "/home/user/project", false},
		{"/home/user/projectevil/file.txt", "/home/user/project", false},
	}
	for _, tt := range tests {
		got := isPathWithin(tt.path, tt.parent)
		if got != tt.want {
			t.Errorf("isPathWithin(%q, %q) = %v, want %v", tt.path, tt.parent, got, tt.want)
		}
	}
}

func TestSetGetMode(t *testing.T) {
	pm := NewEnhancedPermissionManager(PermReadOnly, "/tmp")
	if pm.GetMode() != PermReadOnly {
		t.Errorf("GetMode = %q, want %q", pm.GetMode(), PermReadOnly)
	}
	pm.SetMode(PermDangerFullAccess)
	if pm.GetMode() != PermDangerFullAccess {
		t.Errorf("GetMode after SetMode = %q, want %q", pm.GetMode(), PermDangerFullAccess)
	}
}

func TestIsToolAllowedDenied(t *testing.T) {
	pm := NewEnhancedPermissionManager(PermWorkspaceWrite, "/tmp")
	pm.ToolAllowList["bash"] = true
	pm.ToolDenyList["write_file"] = true

	if !pm.IsToolAllowed("bash") {
		t.Error("bash should be allowed")
	}
	if pm.IsToolAllowed("write_file") {
		t.Error("write_file should not be in allow list")
	}
	if !pm.IsToolDenied("write_file") {
		t.Error("write_file should be denied")
	}
}

func TestOSName(t *testing.T) {
	os := OSName()
	if os == "" {
		t.Error("OSName should not be empty")
	}
}

// --- Concurrent access test ---

func TestEnhancedPermissionManagerConcurrentAccess(t *testing.T) {
	pm := NewEnhancedPermissionManager(PermPrompt, "/tmp")

	done := make(chan bool, 100)

	for i := 0; i < 50; i++ {
		go func() {
			defer func() { done <- true }()
			input := json.RawMessage(`{"command":"test"}`)
			pm.CheckToolPermission("bash", PermExecuteCommand, input)
		}()
	}

	for i := 0; i < 50; i++ {
		go func() {
			defer func() { done <- true }()
			input := json.RawMessage(`{"command":"test"}`)
			pm.RecordDecision("bash", input, true)
		}()
	}

	for i := 0; i < 100; i++ {
		<-done
	}
}

// --- Integration: write_file path validation across modes ---

func TestWriteFilePathValidationIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a subdirectory to ensure proper resolution.
	subDir := filepath.Join(tmpDir, "project")
	os.MkdirAll(subDir, 0o755)

	insidePath := filepath.Join(subDir, "file.txt")
	outsidePath := filepath.Join(tmpDir, "outside.txt")

	modes := []PermissionMode{PermWorkspaceWrite, PermDangerFullAccess, PermPrompt}
	for _, mode := range modes {
		t.Run(string(mode), func(t *testing.T) {
			pm := NewEnhancedPermissionManager(mode, subDir)

			// Inside workspace: should be allowed (or needs_prompt for prompt mode).
			input := json.RawMessage(`{"path":"` + insidePath + `","content":"data"}`)
			result := pm.CheckToolPermission("write_file", PermWriteFile, input)
			if mode == PermPrompt {
				// Prompt mode should return needs_prompt for uncached requests.
				if result.Allowed {
					t.Error("prompt mode should not auto-allow")
				}
				if result.DenialReason != "needs_prompt" {
					t.Errorf("DenialReason = %q, want needs_prompt", result.DenialReason)
				}
			} else {
				if !result.Allowed {
					t.Error("inside workspace write should be allowed")
				}
			}

			// Outside workspace: should always be denied.
			input = json.RawMessage(`{"path":"` + outsidePath + `","content":"data"}`)
			result = pm.CheckToolPermission("write_file", PermWriteFile, input)
			if result.Allowed {
				t.Errorf("outside workspace write should be denied in %s mode", mode)
			}
		})
	}
}
