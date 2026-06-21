// Package tools re-exports types from internal/modules/tools/infrastructure/registry.
// The canonical implementation lives in the modules structure.
package tools

import registry "github.com/hieu-glaw/glaw-code/internal/modules/tools/infrastructure/registry"

// Re-export all types
type Registry = registry.Registry

// Re-export functions
var NewRegistry = registry.NewRegistry
