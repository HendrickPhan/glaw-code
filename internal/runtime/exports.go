// Package runtime provides the conversation runtime and related types.
//
// This package re-exports types from the modular structure under
// internal/modules/. The canonical implementations live in:
//
//   - internal/modules/session/       — Session entity and persistence
//   - internal/modules/permission/    — Permission management
//   - internal/modules/conversation/  — Conversation runtime, config, usage tracking
//
// Existing code importing "internal/runtime" continues to work unchanged.
package runtime

import (
	sessionentity "github.com/hieu-glaw/glaw-code/internal/modules/session/domain/entity"
	sessionpersistence "github.com/hieu-glaw/glaw-code/internal/modules/session/infrastructure/persistence"
	permentity "github.com/hieu-glaw/glaw-code/internal/modules/permission/domain/entity"
	permservice "github.com/hieu-glaw/glaw-code/internal/modules/permission/domain/service"
	conventity "github.com/hieu-glaw/glaw-code/internal/modules/conversation/domain/entity"
	convinfra "github.com/hieu-glaw/glaw-code/internal/modules/conversation/infrastructure"
)

// =====================================================================
// Re-exported types from modules/session
// =====================================================================

// ConversationMessage represents a single message in a conversation.
type ConversationMessage = sessionentity.ConversationMessage

// Session represents a conversation session.
type Session = sessionentity.Session

// NewSession creates a new empty session.
var NewSession = sessionentity.NewSession

// =====================================================================
// Re-exported types from modules/permission
// =====================================================================

// PermissionMode defines the permission level for tool execution.
type PermissionMode = permentity.PermissionMode

// Permission represents a specific permission type.
type Permission = permentity.Permission

// PermissionResult holds the outcome of a permission check.
type PermissionResult = permentity.PermissionResult

// CacheDecision indicates how the result was determined.
type CacheDecision = permentity.CacheDecision

const (
	PermReadOnly         = permentity.PermReadOnly
	PermWorkspaceWrite   = permentity.PermWorkspaceWrite
	PermDangerFullAccess = permentity.PermDangerFullAccess
	PermPrompt           = permentity.PermPrompt
	PermAllow            = permentity.PermAllow
	PermYolo             = permentity.PermYolo

	PermReadFile       = permentity.PermReadFile
	PermWriteFile      = permentity.PermWriteFile
	PermEditFile       = permentity.PermEditFile
	PermExecuteCommand = permentity.PermExecuteCommand
	PermNetwork        = permentity.PermNetwork

	CacheNone       = permentity.CacheNone
	CacheHitAllowed = permentity.CacheHitAllowed
	CacheHitDenied  = permentity.CacheHitDenied
)

// PermissionManager handles permission checking.
type PermissionManager = permservice.PermissionManager

// EnhancedPermissionManager handles enhanced permission checking.
type EnhancedPermissionManager = permservice.EnhancedPermissionManager

// NewPermissionManager creates a new permission manager.
var NewPermissionManager = permservice.NewPermissionManager

// NewEnhancedPermissionManager creates a new enhanced permission manager.
var NewEnhancedPermissionManager = permservice.NewEnhancedPermissionManager

// NewEnhancedPermissionManagerFromSettings creates from config settings.
var NewEnhancedPermissionManagerFromSettings = permservice.NewEnhancedPermissionManagerFromSettings

// ValidatePathWithinWorkspace checks that a path is within the workspace root.
var ValidatePathWithinWorkspace = permservice.ValidatePathAbsWithinWorkspace

// =====================================================================
// Re-exported types from modules/conversation
// =====================================================================

// Config holds application configuration.
type Config = conventity.Config

// ToolOutput represents the result of a tool execution.
type ToolOutput = conventity.ToolOutput

// ToolExecutor is the interface for executing tools.
type ToolExecutor = conventity.ToolExecutor

// SubAgentSessionProvider is the interface for sub-agent session info.
type SubAgentSessionProvider = conventity.SubAgentSessionProvider

// ConversationRuntime is the central orchestrator.
type ConversationRuntime = conventity.ConversationRuntime// TurnResult holds the result of a single agentic turn.
type TurnResult = conventity.TurnResult

// ActionCancelledError is returned when the user cancels a running action.
type ActionCancelledError = conventity.ActionCancelledError

// ModelPricing holds per-token cost information.
type ModelPricing = conventity.ModelPricing

// UsageTracker tracks token usage across turns.
type UsageTracker = conventity.UsageTracker

// FileSnapshot captures a file's state before a modification.
type FileSnapshot = conventity.FileSnapshot

// SnapshotBatch groups snapshots from a single user interaction.
type SnapshotBatch = conventity.SnapshotBatch

// SnapshottingExecutor wraps a ToolExecutor for undo support.
type SnapshottingExecutor = conventity.SnapshottingExecutor

// CompositeToolExecutor delegates to builtin + MCP tools.
type CompositeToolExecutor = conventity.CompositeToolExecutor

// SystemPromptBuilder constructs system prompts.
type SystemPromptBuilder = conventity.SystemPromptBuilder

// DefaultConfig returns sensible defaults.
var DefaultConfig = conventity.DefaultConfig

// ConfigFromSettings converts a config.Settings into a Config.
var ConfigFromSettings = conventity.ConfigFromSettings

// NewConversationRuntime creates a new runtime.
var NewConversationRuntime = conventity.NewConversationRuntime

// IsActionCancelled returns true if the error is due to user cancellation.
var IsActionCancelled = conventity.IsActionCancelled

// NewUsageTracker creates a new usage tracker.
var NewUsageTracker = conventity.NewUsageTracker

// PricingForModel returns pricing for the given model.
var PricingForModel = conventity.PricingForModel

// FormatUSD formats a float as USD string.
var FormatUSD = conventity.FormatUSD

// NewSnapshottingExecutor creates a new snapshotting wrapper.
var NewSnapshottingExecutor = conventity.NewSnapshottingExecutor

// NewCompositeToolExecutor creates a composite executor.
var NewCompositeToolExecutor = conventity.NewCompositeToolExecutor

// NewSystemPromptBuilder creates a new builder.
var NewSystemPromptBuilder = conventity.NewSystemPromptBuilder

// LoadInstructionFiles discovers and loads instruction files.
var LoadInstructionFiles = conventity.LoadInstructionFiles

// =====================================================================
// Re-exported types from modules/conversation/infrastructure
// =====================================================================

// SandboxStatus describes the current sandbox environment.
type SandboxStatus = convinfra.SandboxStatus

// SandboxCommandBuilder constructs sandboxed command invocations.
type SandboxCommandBuilder = convinfra.SandboxCommandBuilder

// SecurityInvariant represents a security rule.
type SecurityInvariant = convinfra.SecurityInvariant

// DetectSandboxStatus probes the current environment.
var DetectSandboxStatus = convinfra.DetectSandboxStatus

// NewSandboxCommandBuilder creates a builder.
var NewSandboxCommandBuilder = convinfra.NewSandboxCommandBuilder

// SecurityChecks returns the list of security invariants.
var SecurityChecks = convinfra.SecurityChecks

// RunSecurityChecks executes all security invariant checks.
var RunSecurityChecks = convinfra.RunSecurityChecks

// ValidateGitSafety checks that git operations follow safety conventions.
var ValidateGitSafety = convinfra.ValidateGitSafety

// EnsureExplicitStaging validates explicit git staging.
var EnsureExplicitStaging = convinfra.EnsureExplicitStaging

// OSName returns the current operating system name.
var OSName = convinfra.OSName

// =====================================================================
// Session persistence (delegates to modules/session/infrastructure)
// =====================================================================

// SaveSession persists a session to disk.
func SaveSession(session *Session, dir string) (string, error) {
	return sessionpersistence.SaveSession((*sessionentity.Session)(session), dir)
}

// LoadSession reads a session from disk.
func LoadSession(path string) (*Session, error) {
	return sessionpersistence.LoadSession(path)
}
