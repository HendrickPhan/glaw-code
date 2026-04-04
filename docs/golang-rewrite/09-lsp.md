# LSP (Language Server Protocol) Integration

## Package: `internal/lsp/`

---

## Overview

The LSP module provides code intelligence features by managing connections to language servers. It supports multiple servers simultaneously, each handling different file extensions.

---

## LspServerConfig

```go
type LspServerConfig struct {
    Name                string
    Command             string            // e.g. "gopls"
    Args                []string          // e.g. ["-mode", "stdio"]
    Env                 map[string]string
    WorkspaceRoot       string
    InitializationOptions json.RawMessage
    ExtensionToLanguage map[string]string // e.g. ".go" -> "go", ".py" -> "python"
}
```

---

## LspManager

```go
type LspManager struct {
    ServerConfigs map[string]LspServerConfig  // name -> config
    ExtensionMap  map[string]string           // ".go" -> server_name
    Clients       map[string]*LspClient       // server_name -> client (lazy init)
}
```

### Constructor

```go
func NewLspManager(configs []LspServerConfig) (*LspManager, error)
```

**Validation:**
```
1. Build extension map: for each config, map all extensions to server name
2. Check for duplicate extension mappings
3. IF duplicate found: return DuplicateExtension error
4. Return initialized LspManager (clients not started yet)
```

### Lazy Client Initialization

```go
func (m *LspManager) clientForPath(ctx context.Context, path string) (*LspClient, error)
```

**Logic:**
```
1. Get file extension from path (e.g. ".go")
2. Normalize: ensure leading dot, lowercase
3. Look up server name in ExtensionMap
4. IF not found: return UnsupportedDocument error
5. Check if client already exists in Clients map
6. IF exists: return existing client
7. Create new LspClient:
   a. Get server config by name
   b. Call LspClient.Connect(config)
   c. Store in Clients map
   d. Return client
```

---

## LspClient

```go
type LspClient struct {
    Config         LspServerConfig
    Process        *os.Process
    Stdin          io.WriteCloser
    Stdout         *bufio.Reader
    NextRequestID  int
    PendingReqs    map[int]chan JsonRpcResponse
    Diagnostics    map[string][]Diagnostic
    OpenDocuments  map[string]bool
}
```

### Connect

```go
func (c *LspClient) Connect(ctx context.Context, config LspServerConfig) error
```

**Logic:**
```
1. Create exec.Cmd with config.Command + config.Args
2. Set environment (OS env + config.Env)
3. Create stdin/stdout pipes
4. Start process
5. Start background reader goroutine (see below)
6. Send LSP initialize request:
   {
     "processId": os.Getpid(),
     "rootUri": "file://{workspace_root}",
     "capabilities": {
       "textDocument": {
         "definition": {"dynamicRegistration": false},
         "references": {"dynamicRegistration": false},
         "publishDiagnostics": {"relatedInformation": true}
       }
     }
   }
7. Wait for initialize response
8. Send initialized notification
9. Return nil on success
```

### Background Reader Goroutine

```
LOOP:
  1. Read Content-Length header from stdout
  2. Read JSON body of specified length
  3. Parse as JSON-RPC response
  4. IF response has ID:
     - Look up pending request channel by ID
     - Send response to channel (unblock waiting caller)
  5. IF response is notification (no ID):
     - IF method == "textDocument/publishDiagnostics":
       Parse params, update Diagnostics map
     - ELSE: ignore
```

---

## Document Lifecycle

### OpenDocument

```go
func (c *LspClient) OpenDocument(ctx context.Context, path string) error
```

**Logic:**
```
1. Read file content from disk
2. Detect language from extension
3. Send textDocument/didOpen notification:
   {
     "textDocument": {
       "uri": "file://{path}",
       "languageId": "{language}",
       "version": 0,
       "text": "{content}"
     }
   }
4. Add to OpenDocuments set
```

### ChangeDocument

```go
func (c *LspClient) ChangeDocument(ctx context.Context, path string, content string) error
```

**Logic:**
```
1. Send textDocument/didChange notification:
   {
     "textDocument": {
       "uri": "file://{path}",
       "version": {incrementing_version}
     },
     "contentChanges": [
       {"text": "{full_content}"}
     ]
   }
```

### SaveDocument

```go
func (c *LspClient) SaveDocument(ctx context.Context, path string) error
```

**Logic:** Send `textDocument/didSave` notification with file URI.

### CloseDocument

```go
func (c *LspClient) CloseDocument(ctx context.Context, path string) error
```

**Logic:**
```
1. Send textDocument/didClose notification with file URI
2. Remove from OpenDocuments set
3. Clear diagnostics for this file
```

---

## Navigation Operations

### GoToDefinition

```go
func (c *LspClient) GoToDefinition(ctx context.Context, path string, line, character int) ([]SymbolLocation, error)
```

**Logic:**
```
1. Ensure document is open (call EnsureDocumentOpen)
2. Send textDocument/definition request:
   {
     "textDocument": {"uri": "file://{path}"},
     "position": {"line": {line}, "character": {character}}
   }
3. Parse response:
   - IF null: return empty list
   - IF single Location: extract URI + range
   - IF array of Location: extract all
   - IF LocationLink: extract targetUri + targetRange
4. Convert URIs to file paths
5. Return SymbolLocation list
```

### FindReferences

```go
func (c *LspClient) FindReferences(ctx context.Context, path string, line, character int) ([]SymbolLocation, error)
```

**Logic:**
```
1. Ensure document is open
2. Send textDocument/references request:
   {
     "textDocument": {"uri": "file://{path}"},
     "position": {"line": {line}, "character": {character}},
     "context": {"includeDeclaration": true}
   }
3. Parse response as array of Location
4. Convert URIs to file paths
5. Return SymbolLocation list
```

---

## Diagnostic Collection

### CollectWorkspaceDiagnostics

```go
func (m *LspManager) CollectWorkspaceDiagnostics(ctx context.Context) (*WorkspaceDiagnostics, error)
```

**Logic:**
```
1. FOR EACH active client in Clients map:
     Get diagnostics snapshot from client.Diagnostics
     FOR EACH (uri, diagnostics) pair:
       Convert URI to file path
       Add to WorkspaceDiagnostics
2. Return aggregated diagnostics
```

---

## Context Enrichment

The LSP module enriches the system prompt with code intelligence:

```go
func (m *LspManager) ContextEnrichment(ctx context.Context, filePath string) (*LspContextEnrichment, error)
```

**Logic:**
```
1. Get diagnostics for the file
2. Get definitions available in the file (if cursor position known)
3. Get references for symbols under cursor
4. Build LspContextEnrichment struct
5. Render as markdown section for system prompt:
   IF !enrichment.IsEmpty():
     Return formatted section with:
     - Up to 12 diagnostics
     - Up to 12 definition/reference locations
   ELSE:
     Return ""
```

### Render Format

```markdown
## LSP Context

### Diagnostics (N)
- **{path}:{line}:{col}**: {message} [{severity}]
- ...

### Definitions (N)
- {path}:{line}:{col}
- ...

### References (N)
- {path}:{line}:{col}
- ...
```

---

## Shutdown

```go
func (c *LspClient) Shutdown(ctx context.Context) error
```

**Logic:**
```
1. Send shutdown request (no params)
2. Wait for response
3. Send exit notification (no params)
4. Kill process (os.Process.Kill)
5. Close stdin pipe
6. Clean up resources
```

```go
func (m *LspManager) Shutdown(ctx context.Context) error
```

**Logic:**
```
1. FOR EACH active client:
     Call client.Shutdown()
2. Clear Clients map
3. Return first error encountered (if any)
```

---

## Helper: Deduplication

```go
func dedupeLocations(locs []SymbolLocation) []SymbolLocation
```

**Logic:**
```
1. Create seen set of "path:line:character" strings
2. Filter locations: only include if not in seen set
3. Return deduplicated list
```
