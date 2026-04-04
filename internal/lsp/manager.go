package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// Manager manages multiple LSP server connections.
type Manager struct {
	configs map[string]ServerConfig // server name -> config
	extMap  map[string]string       // ".go" -> server name
	clients map[string]*Client      // server name -> client (lazy init)
	mu      sync.RWMutex
}

// NewManager creates a new LSP manager.
func NewManager(configs []ServerConfig) (*Manager, error) {
	m := &Manager{
		configs: make(map[string]ServerConfig),
		extMap:  make(map[string]string),
		clients: make(map[string]*Client),
	}

	for _, cfg := range configs {
		m.configs[cfg.Name] = cfg
		for ext := range cfg.ExtensionToLanguage {
			normalized := NormalizeExtension(ext)
			if existing, ok := m.extMap[normalized]; ok {
				return nil, &Error{
					Message: fmt.Sprintf("duplicate extension mapping: %q mapped to both %q and %q",
						normalized, existing, cfg.Name),
				}
			}
			m.extMap[normalized] = cfg.Name
		}
	}

	return m, nil
}

// SupportsPath checks if any server handles the given file's extension.
func (m *Manager) SupportsPath(path string) bool {
	ext := filepath.Ext(path)
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.extMap[NormalizeExtension(ext)]
	return ok
}

// GoToDefinition finds where a symbol is defined.
func (m *Manager) GoToDefinition(ctx context.Context, filePath string, line, character int) ([]SymbolLocation, error) {
	client, err := m.clientForPath(ctx, filePath)
	if err != nil {
		return nil, err
	}
	locs, err := client.GoToDefinition(ctx, filePath, line, character)
	if err != nil {
		return nil, err
	}
	return dedupeLocations(locs), nil
}

// FindReferences finds all references to a symbol.
func (m *Manager) FindReferences(ctx context.Context, filePath string, line, character int) ([]SymbolLocation, error) {
	client, err := m.clientForPath(ctx, filePath)
	if err != nil {
		return nil, err
	}
	locs, err := client.FindReferences(ctx, filePath, line, character)
	if err != nil {
		return nil, err
	}
	return dedupeLocations(locs), nil
}

// CollectWorkspaceDiagnostics aggregates diagnostics from all active servers.
func (m *Manager) CollectWorkspaceDiagnostics(ctx context.Context) (*WorkspaceDiagnostics, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var wd WorkspaceDiagnostics
	for _, client := range m.clients {
		diags := client.DiagnosticsSnapshot()
		for path, ds := range diags {
			wd.Files = append(wd.Files, FileDiagnostics{
				Path:        path,
				URI:         "file://" + path,
				Diagnostics: ds,
			})
		}
	}
	return &wd, nil
}

// ContextEnrichment provides LSP context for a file.
func (m *Manager) ContextEnrichment(ctx context.Context, filePath string, line, character int) (*ContextEnrichment, error) {
	enrichment := &ContextEnrichment{FilePath: filePath}

	// Get diagnostics
	m.mu.RLock()
	for _, client := range m.clients {
		diags := client.DiagnosticsSnapshot()
		if ds, ok := diags[filePath]; ok {
			enrichment.Diagnostics = ds
		}
	}
	m.mu.RUnlock()

	// Get definitions and references if position provided
	if line >= 0 && character >= 0 {
		m.mu.RLock()
		supported := m.SupportsPath(filePath)
		m.mu.RUnlock()
		if supported {
			if defs, err := m.GoToDefinition(ctx, filePath, line, character); err == nil {
				enrichment.Definitions = defs
			}
			if refs, err := m.FindReferences(ctx, filePath, line, character); err == nil {
				enrichment.References = refs
			}
		}
	}

	return enrichment, nil
}

// Hover returns hover information for a symbol.
func (m *Manager) Hover(ctx context.Context, filePath string, line, character int) (string, error) {
	client, err := m.clientForPath(ctx, filePath)
	if err != nil {
		return "", err
	}
	return client.Hover(ctx, filePath, line, character)
}

// DocumentSymbol returns symbols within a document.
func (m *Manager) DocumentSymbol(ctx context.Context, filePath string) ([]SymbolLocation, error) {
	client, err := m.clientForPath(ctx, filePath)
	if err != nil {
		return nil, err
	}
	return client.DocumentSymbol(ctx, filePath)
}

// WorkspaceSymbol searches for symbols across the workspace.
func (m *Manager) WorkspaceSymbol(ctx context.Context, query string) ([]SymbolLocation, error) {
	m.mu.RLock()
	// Pick the first available client for workspace queries
	var client *Client
	for _, c := range m.clients {
		client = c
		break
	}
	m.mu.RUnlock()

	if client == nil {
		return nil, &Error{Message: "no LSP server connected"}
	}
	return client.WorkspaceSymbol(ctx, query)
}

// GoToImplementation finds implementations of a symbol.
func (m *Manager) GoToImplementation(ctx context.Context, filePath string, line, character int) ([]SymbolLocation, error) {
	client, err := m.clientForPath(ctx, filePath)
	if err != nil {
		return nil, err
	}
	return client.GoToImplementation(ctx, filePath, line, character)
}

// IncomingCalls returns callers of the symbol at the given position.
func (m *Manager) IncomingCalls(ctx context.Context, filePath string, line, character int) ([]CallHierarchyItem, error) {
	client, err := m.clientForPath(ctx, filePath)
	if err != nil {
		return nil, err
	}
	items, err := client.PrepareCallHierarchy(ctx, filePath, line, character)
	if err != nil || len(items) == 0 {
		return nil, err
	}
	return client.IncomingCalls(ctx, items[0])
}

// OutgoingCalls returns callees of the symbol at the given position.
func (m *Manager) OutgoingCalls(ctx context.Context, filePath string, line, character int) ([]CallHierarchyItem, error) {
	client, err := m.clientForPath(ctx, filePath)
	if err != nil {
		return nil, err
	}
	items, err := client.PrepareCallHierarchy(ctx, filePath, line, character)
	if err != nil || len(items) == 0 {
		return nil, err
	}
	return client.OutgoingCalls(ctx, items[0])
}

// ServerStatus represents the status of an LSP server.
type ServerStatus struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	Running bool   `json:"running"`
}

// Status returns the status of all configured LSP servers.
func (m *Manager) Status() []ServerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var statuses []ServerStatus
	for name, cfg := range m.configs {
		_, running := m.clients[name]
		statuses = append(statuses, ServerStatus{
			Name:    name,
			Command: cfg.Command,
			Running: running,
		})
	}
	return statuses
}

// SupportedExtensions returns all file extensions with LSP support.
func (m *Manager) SupportedExtensions() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	exts := make([]string, 0, len(m.extMap))
	for ext := range m.extMap {
		exts = append(exts, ext)
	}
	return exts
}

// Shutdown closes all LSP server connections.
func (m *Manager) Shutdown() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for name, client := range m.clients {
		if err := client.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("closing LSP server %s: %w", name, err)
		}
		delete(m.clients, name)
	}
	return firstErr
}

// clientForPath lazily initializes and returns the client for a file path.
func (m *Manager) clientForPath(ctx context.Context, filePath string) (*Client, error) {
	ext := filepath.Ext(filePath)
	serverName, ok := m.extMap[NormalizeExtension(ext)]
	if !ok {
		return nil, &Error{Message: fmt.Sprintf("no LSP server for extension %q", ext)}
	}

	m.mu.RLock()
	client, ok := m.clients[serverName]
	m.mu.RUnlock()
	if ok {
		return client, nil
	}

	// Lazy init
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check
	if client, ok = m.clients[serverName]; ok {
		return client, nil
	}

	config, ok := m.configs[serverName]
	if !ok {
		return nil, &Error{Message: fmt.Sprintf("no config for LSP server %q", serverName)}
	}

	client = NewClient(config)
	if err := client.Connect(ctx); err != nil {
		return nil, err
	}

	m.clients[serverName] = client
	return client, nil
}

// DedupeLocations removes duplicate locations.
func DedupeLocations(locs []SymbolLocation) []SymbolLocation {
	return dedupeLocations(locs)
}

// AutoDetect searches for known language servers on PATH and returns configs.
func AutoDetect(workspaceRoot string) []ServerConfig {
	var configs []ServerConfig

	knownServers := []struct {
		name    string
		cmd     string
		args    []string
		extLang map[string]string
	}{
		{
			name:    "gopls",
			cmd:     "gopls",
			args:    []string{"serve"},
			extLang: map[string]string{".go": "go"},
		},
		{
			name:    "typescript-language-server",
			cmd:     "typescript-language-server",
			args:    []string{"--stdio"},
			extLang: map[string]string{".ts": "typescript", ".tsx": "typescriptreact", ".js": "javascript", ".jsx": "javascriptreact"},
		},
		{
			name:    "pyright",
			cmd:     "pyright-langserver",
			args:    []string{"--stdio"},
			extLang: map[string]string{".py": "python"},
		},
		{
			name:    "rust-analyzer",
			cmd:     "rust-analyzer",
			args:    []string{},
			extLang: map[string]string{".rs": "rust"},
		},
		{
			name:    "clangd",
			cmd:     "clangd",
			args:    []string{},
			extLang: map[string]string{".c": "c", ".cpp": "cpp", ".h": "c", ".hpp": "cpp"},
		},
	}

	for _, srv := range knownServers {
		if _, err := exec.LookPath(srv.cmd); err == nil {
			configs = append(configs, ServerConfig{
				Name:                srv.name,
				Command:             srv.cmd,
				Args:                srv.args,
				WorkspaceRoot:       workspaceRoot,
				ExtensionToLanguage: srv.extLang,
			})
		}
	}

	return configs
}

// NewAutoDetectedManager creates a Manager by auto-detecting available language servers.
// It returns the manager and the list of detected server configs.
func NewAutoDetectedManager(workspaceRoot string) (*Manager, []ServerConfig, error) {
	// Check for explicit config first
	configPath := filepath.Join(workspaceRoot, ".glaw", "lsp.json")
	if _, err := os.Stat(configPath); err == nil {
		configs, err := loadLSPConfig(configPath, workspaceRoot)
		if err != nil {
			return nil, nil, fmt.Errorf("reading LSP config: %w", err)
		}
		if len(configs) > 0 {
			mgr, err := NewManager(configs)
			return mgr, configs, err
		}
	}

	// Auto-detect
	configs := AutoDetect(workspaceRoot)
	if len(configs) == 0 {
		mgr, err := NewManager(nil)
		return mgr, nil, err
	}
	mgr, err := NewManager(configs)
	return mgr, configs, err
}

// loadLSPConfig reads an LSP configuration file.
func loadLSPConfig(path string, workspaceRoot string) ([]ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Servers []ServerConfig `json:"servers"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	for i := range raw.Servers {
		if raw.Servers[i].WorkspaceRoot == "" {
			raw.Servers[i].WorkspaceRoot = workspaceRoot
		}
	}

	return raw.Servers, nil
}
