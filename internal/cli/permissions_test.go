package cli

import (
	"encoding/json"
	"testing"

	"github.com/hieu-glaw/glaw-code/internal/runtime"
)

func TestFormatToolInput(t *testing.T) {
	tests := []struct {
		name     string
		tool     string
		input    string
		contains string
	}{
		{
			"bash command",
			"bash",
			`{"command":"ls -la"}`,
			"ls -la",
		},
		{
			"bash empty",
			"bash",
			`{}`,
			"empty command",
		},
		{
			"write_file",
			"write_file",
			`{"path":"test.txt","content":"hello world"}`,
			"test.txt",
		},
		{
			"edit_file",
			"edit_file",
			`{"path":"main.go","old_string":"foo","new_string":"bar"}`,
			"main.go",
		},
		{
			"read_file",
			"read_file",
			`{"path":"/tmp/test.txt"}`,
			"/tmp/test.txt",
		},
		{
			"unknown tool",
			"custom_tool",
			`{"key":"value"}`,
			"key",
		},
		{
			"empty input",
			"bash",
			"",
			"no input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input json.RawMessage
			if tt.input != "" {
				input = json.RawMessage(tt.input)
			}
			result := FormatToolInput(tt.tool, input)
			if !contains(result, tt.contains) {
				t.Errorf("FormatToolInput(%q) = %q, want to contain %q", tt.tool, result, tt.contains)
			}
		})
	}
}

func TestFormatToolInputTruncation(t *testing.T) {
	// Test that long commands get truncated.
	longCmd := ""
	for i := 0; i < 500; i++ {
		longCmd += "x"
	}
	input := json.RawMessage(`{"command":"` + longCmd + `"}`)
	result := FormatToolInput("bash", input)
	if len(result) > 400 {
		t.Errorf("long bash command should be truncated, got %d chars", len(result))
	}
}

func TestFormatToolInputWriteFileTruncation(t *testing.T) {
	// Test that long content gets truncated.
	longContent := ""
	for i := 0; i < 500; i++ {
		longContent += "a"
	}
	input := json.RawMessage(`{"path":"big.txt","content":"` + longContent + `"}`)
	result := FormatToolInput("write_file", input)
	if len(result) > 400 {
		t.Errorf("long write_file content should be truncated, got %d chars", len(result))
	}
}

func TestFormatGenericInputTruncation(t *testing.T) {
	// Test generic input truncation.
	input := json.RawMessage(`{"key":"` + string(make([]byte, 600)) + `"}`)
	result := FormatToolInput("unknown_tool", input)
	if len(result) > 600 {
		t.Errorf("generic input should be truncated, got %d chars", len(result))
	}
}

// --- CheckAndPrompt tests ---

func TestCheckAndPromptNilManager(t *testing.T) {
	result := CheckAndPrompt(nil, nil, "bash", runtime.PermExecuteCommand, json.RawMessage(`{}`))
	if !result.Allowed {
		t.Error("nil manager should allow everything")
	}
}

func TestCheckAndPromptFullAccess(t *testing.T) {
	pm := runtime.NewEnhancedPermissionManager(runtime.PermDangerFullAccess, "/tmp")
	result := CheckAndPrompt(pm, nil, "bash", runtime.PermExecuteCommand, json.RawMessage(`{"command":"ls"}`))
	if !result.Allowed {
		t.Error("full access mode should allow bash")
	}
}

func TestCheckAndPromptReadOnlyDenied(t *testing.T) {
	pm := runtime.NewEnhancedPermissionManager(runtime.PermReadOnly, "/tmp")
	result := CheckAndPrompt(pm, nil, "bash", runtime.PermExecuteCommand, json.RawMessage(`{"command":"ls"}`))
	if result.Allowed {
		t.Error("read_only mode should deny bash")
	}
}

func TestCheckAndPromptPromptModeNoPrompter(t *testing.T) {
	pm := runtime.NewEnhancedPermissionManager(runtime.PermPrompt, "/tmp")
	result := CheckAndPrompt(pm, nil, "bash", runtime.PermExecuteCommand, json.RawMessage(`{"command":"ls"}`))
	if result.Allowed {
		t.Error("prompt mode without prompter should deny")
	}
	if result.DenialReason != "no_prompter" {
		t.Errorf("DenialReason = %q, want %q", result.DenialReason, "no_prompter")
	}
}

func TestCheckAndPromptCachedApproval(t *testing.T) {
	pm := runtime.NewEnhancedPermissionManager(runtime.PermPrompt, "/tmp")
	input := json.RawMessage(`{"command":"ls"}`)

	// Cache an approval.
	pm.RecordDecision("bash", input, true)

	// CheckAndPrompt should return the cached approval without needing a prompter.
	result := CheckAndPrompt(pm, nil, "bash", runtime.PermExecuteCommand, input)
	if !result.Allowed {
		t.Error("cached approval should be returned without prompting")
	}
}

// --- PromptChoice constants test ---

func TestPromptChoiceValues(t *testing.T) {
	if ChoiceDeny != 0 {
		t.Errorf("ChoiceDeny = %d, want 0", ChoiceDeny)
	}
	if ChoiceOnce != 1 {
		t.Errorf("ChoiceOnce = %d, want 1", ChoiceOnce)
	}
	if ChoiceAlways != 2 {
		t.Errorf("ChoiceAlways = %d, want 2", ChoiceAlways)
	}
}

// --- Helper ---

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
