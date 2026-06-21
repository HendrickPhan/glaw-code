// Package lsp re-exports types from internal/modules/lsp.
// The canonical implementation lives in the modules structure.
package lsp

import (
	lsentity "github.com/hieu-glaw/glaw-code/internal/modules/lsp/domain/entity"
	lsclient "github.com/hieu-glaw/glaw-code/internal/modules/lsp/infrastructure/client"
)

// Re-export domain entity types
type ServerConfig = lsentity.ServerConfig
type Diagnostic = lsentity.Diagnostic
type Range = lsentity.Range
type Position = lsentity.Position
type SymbolLocation = lsentity.SymbolLocation
type FileDiagnostics = lsentity.FileDiagnostics
type WorkspaceDiagnostics = lsentity.WorkspaceDiagnostics
type ContextEnrichment = lsentity.ContextEnrichment

// Re-export infrastructure types
type Error = lsclient.Error
type Client = lsclient.Client
type Manager = lsclient.Manager
type CallHierarchyItem = lsclient.CallHierarchyItem
type ServerStatus = lsclient.ServerStatus

// Re-export functions
var NewClient = lsclient.NewClient
var NewManager = lsclient.NewManager
var NormalizeExtension = lsentity.NormalizeExtension
var SeverityString = lsentity.SeverityString
var AutoDetect = lsclient.AutoDetect
var NewAutoDetectedManager = lsclient.NewAutoDetectedManager
var DedupeLocations = lsclient.DedupeLocations
