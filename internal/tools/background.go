// Package tools re-exports from internal/modules/tools/infrastructure/registry.
package tools

import registry "github.com/hieu-glaw/glaw-code/internal/modules/tools/infrastructure/registry"

// Re-export background command types
type BackgroundCommand = registry.BackgroundCommand
type BackgroundCommandManager = registry.BackgroundCommandManager

// Re-export functions
var NewBackgroundCommandManager = registry.NewBackgroundCommandManager
