// Package lsp re-exports Manager from internal/modules/lsp/infrastructure/client.
package lsp

import lsclient "github.com/hieu-glaw/glaw-code/internal/modules/lsp/infrastructure/client"

// This file ensures the Manager type is accessible from the lsp facade.
// The full implementation lives in infrastructure/client.
var _ = lsclient.NewManager
