// Package config re-exports types from internal/modules/config/domain/entity.
// The canonical implementation lives in the modules structure.
// Existing code importing "internal/config" continues to work unchanged.
package config

import entity "github.com/hieu-glaw/glaw-code/internal/modules/config/domain/entity"

// Re-export all types
type PermissionSettings = entity.PermissionSettings
type PluginSettings = entity.PluginSettings
type MCPServerConfig = entity.MCPServerConfig
type Settings = entity.Settings

// Re-export constants
const DefaultGlobalDir = entity.DefaultGlobalDir
const ProjectDir = entity.ProjectDir
const SettingsFileName = entity.SettingsFileName

// Re-export functions
var DefaultSettings = entity.DefaultSettings
var GlobalConfigDir = entity.GlobalConfigDir
var GlobalSettingsPath = entity.GlobalSettingsPath
var ProjectSettingsPath = entity.ProjectSettingsPath
var LoadFromFile = entity.LoadFromFile
var Merge = entity.Merge
var LoadAll = entity.LoadAll
var Save = entity.Save
var SaveGlobal = entity.SaveGlobal
var SaveProject = entity.SaveProject
var EnsureGlobalDir = entity.EnsureGlobalDir
var LoadClaudeMCP = entity.LoadClaudeMCP
