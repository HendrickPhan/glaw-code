package client

import (
	lsentity "github.com/hieu-glaw/glaw-code/internal/modules/lsp/domain/entity"
)

// Re-export domain entity types for convenience.
type ServerConfig = lsentity.ServerConfig
type Diagnostic = lsentity.Diagnostic
type Range = lsentity.Range
type Position = lsentity.Position
type SymbolLocation = lsentity.SymbolLocation
type FileDiagnostics = lsentity.FileDiagnostics
type WorkspaceDiagnostics = lsentity.WorkspaceDiagnostics
type ContextEnrichment = lsentity.ContextEnrichment
type NormalizeExtensionFunc = func(string) string

// NormalizeExtension re-exports the entity utility.
var NormalizeExtension = lsentity.NormalizeExtension
