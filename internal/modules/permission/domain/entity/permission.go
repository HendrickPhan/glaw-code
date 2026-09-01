package entity

// PermissionMode defines the permission level for tool execution.
type PermissionMode string

const (
	PermReadOnly         PermissionMode = "read_only"
	PermWorkspaceWrite   PermissionMode = "workspace_write"
	PermDangerFullAccess PermissionMode = "danger_full_access"
	PermPrompt           PermissionMode = "prompt"
	PermAllow            PermissionMode = "allow"
	PermYolo             PermissionMode = "yolo"
)

// Permission represents a specific permission type.
type Permission string

const (
	PermReadFile       Permission = "read_file"
	PermWriteFile      Permission = "write_file"
	PermEditFile       Permission = "edit_file"
	PermExecuteCommand Permission = "execute_command"
	PermNetwork        Permission = "network"
)

// PermissionResult holds the outcome of a permission check.
type PermissionResult struct {
	Allowed       bool
	Message       string
	DenialReason  string
	CacheDecision CacheDecision
}

// CacheDecision indicates how the result was determined.
type CacheDecision int

const (
	CacheNone CacheDecision = iota
	CacheHitAllowed
	CacheHitDenied
)
