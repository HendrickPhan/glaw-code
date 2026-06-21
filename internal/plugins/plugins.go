// Package plugins re-exports types from internal/modules/plugins/domain/entity.
// The canonical implementation lives in the modules structure.
package plugins

import entity "github.com/hieu-glaw/glaw-code/internal/modules/plugins/domain/entity"

// Re-export all types
type HookEvent = entity.HookEvent
type HookConfig = entity.HookConfig
type ToolConfig = entity.ToolConfig
type Manifest = entity.Manifest
type Plugin = entity.Plugin
type ToolDefinition = entity.ToolDefinition
type Manager = entity.Manager

// Re-export constants
const HookPreTool = entity.HookPreTool
const HookPostTool = entity.HookPostTool
const HookPreMessage = entity.HookPreMessage
const HookPostMessage = entity.HookPostMessage
const HookSessionStart = entity.HookSessionStart
const HookSessionEnd = entity.HookSessionEnd

// Re-export functions
var NewManager = entity.NewManager
var ValidateManifest = entity.ValidateManifest
