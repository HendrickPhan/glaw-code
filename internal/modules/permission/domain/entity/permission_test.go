package entity

import "testing"

// =====================================================================
// PermissionMode Constants Tests
// =====================================================================

func TestPermissionModeValues(t *testing.T) {
	tests := []struct {
		mode PermissionMode
		want string
	}{
		{PermReadOnly, "read_only"},
		{PermWorkspaceWrite, "workspace_write"},
		{PermDangerFullAccess, "danger_full_access"},
		{PermPrompt, "prompt"},
		{PermAllow, "allow"},
		{PermYolo, "yolo"},
	}
	for _, tt := range tests {
		if string(tt.mode) != tt.want {
			t.Errorf("PermissionMode constant = %q, want %q", string(tt.mode), tt.want)
		}
	}
}

// =====================================================================
// Permission Constants Tests
// =====================================================================

func TestPermissionValues(t *testing.T) {
	tests := []struct {
		perm Permission
		want string
	}{
		{PermReadFile, "read_file"},
		{PermWriteFile, "write_file"},
		{PermEditFile, "edit_file"},
		{PermExecuteCommand, "execute_command"},
		{PermNetwork, "network"},
	}
	for _, tt := range tests {
		if string(tt.perm) != tt.want {
			t.Errorf("Permission constant = %q, want %q", string(tt.perm), tt.want)
		}
	}
}

// =====================================================================
// PermissionResult Tests
// =====================================================================

func TestPermissionResultAllowed(t *testing.T) {
	r := &PermissionResult{Allowed: true, Message: "granted"}
	if !r.Allowed {
		t.Error("Allowed should be true")
	}
	if r.Message != "granted" {
		t.Errorf("Message = %q, want %q", r.Message, "granted")
	}
}

func TestPermissionResultDenied(t *testing.T) {
	r := &PermissionResult{
		Allowed:      false,
		DenialReason: "tool_deny_list",
		Message:      "tool is denied",
	}
	if r.Allowed {
		t.Error("Allowed should be false")
	}
	if r.DenialReason != "tool_deny_list" {
		t.Errorf("DenialReason = %q, want %q", r.DenialReason, "tool_deny_list")
	}
}

func TestPermissionResultCacheDecision(t *testing.T) {
	tests := []struct {
		decision CacheDecision
		name     string
	}{
		{CacheNone, "CacheNone"},
		{CacheHitAllowed, "CacheHitAllowed"},
		{CacheHitDenied, "CacheHitDenied"},
	}
	for _, tt := range tests {
		r := &PermissionResult{CacheDecision: tt.decision}
		if r.CacheDecision != tt.decision {
			t.Errorf("CacheDecision = %d, want %d (%s)", r.CacheDecision, tt.decision, tt.name)
		}
	}
}

// =====================================================================
// CacheDecision Constants Tests
// =====================================================================

func TestCacheDecisionOrder(t *testing.T) {
	// Verify the iota ordering
	if CacheNone >= CacheHitAllowed {
		t.Error("CacheNone should be less than CacheHitAllowed")
	}
	if CacheHitAllowed >= CacheHitDenied {
		t.Error("CacheHitAllowed should be less than CacheHitDenied")
	}
}
