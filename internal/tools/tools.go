package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hieu-glaw/glaw-code/internal/api"
	"github.com/hieu-glaw/glaw-code/internal/config"
	"github.com/hieu-glaw/glaw-code/internal/lsp"
	"github.com/hieu-glaw/glaw-code/internal/runtime"
)

// ToolFunc is a handler for a named tool.
type ToolFunc func(ctx context.Context, input json.RawMessage) (*runtime.ToolOutput, error)

// Registry holds all registered tool definitions and their handlers.
type Registry struct {
	workspaceRoot string
	lspManager    *lsp.Manager
	handlers      map[string]ToolFunc
	specs         []api.ToolDefinition
}

// NewRegistry creates a new tool registry with built-in tools.
func NewRegistry(workspaceRoot string) *Registry {
	r := &Registry{
		workspaceRoot: workspaceRoot,
		handlers:      make(map[string]ToolFunc),
	}
	r.registerBuiltinTools()
	return r
}

// SetLSPManager sets the LSP manager for LSP-related tools.
func (r *Registry) SetLSPManager(m *lsp.Manager) {
	r.lspManager = m
}

// registerBuiltinTools adds all built-in tools to the registry.
func (r *Registry) registerBuiltinTools() {
	tools := []struct {
		spec    api.ToolDefinition
		handler ToolFunc
	}{
		{api.ToolDefinition{
			Name:        "bash",
			Description: "Execute a shell command",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"The bash command to execute"},"timeout":{"type":"integer","description":"Timeout in milliseconds","default":120000}},"required":["command"]}`),
		}, r.bashTool},
		{api.ToolDefinition{
			Name:        "read_file",
			Description: "Read file contents",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Absolute or relative path to the file"}},"required":["path"]}`),
		}, r.readFileTool},
		{api.ToolDefinition{
			Name:        "write_file",
			Description: "Write content to a file, creating parent directories as needed",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Path to write to"},"content":{"type":"string","description":"Content to write"}},"required":["path","content"]}`),
		}, r.writeFileTool},
		{api.ToolDefinition{
			Name:        "edit_file",
			Description: "Edit a file by replacing an exact string match",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Path to the file"},"old_string":{"type":"string","description":"Exact string to find"},"new_string":{"type":"string","description":"Replacement string"}},"required":["path","old_string","new_string"]}`),
		}, r.editFileTool},
		{api.ToolDefinition{
			Name:        "glob_search",
			Description: "Find files matching a glob pattern",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string","description":"Glob pattern (e.g. **/*.go)"}},"required":["pattern"]}`),
		}, r.globSearchTool},
		{api.ToolDefinition{
			Name:        "grep_search",
			Description: "Search file contents for a pattern",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string","description":"Regex pattern to search for"},"path":{"type":"string","description":"Directory or file to search in"}},"required":["pattern"]}`),
		}, r.grepSearchTool},
		{api.ToolDefinition{
			Name:        "web_fetch",
			Description: "Fetch content from a URL via HTTP GET",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"url":{"type":"string","description":"URL to fetch"},"timeout":{"type":"integer","description":"Timeout in seconds","default":30}},"required":["url"]}`),
		}, r.webFetchTool},
		{api.ToolDefinition{
			Name:        "web_search",
			Description: "Search the web using DuckDuckGo HTML scraping",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Search query"},"allowed_domains":{"type":"array","items":{"type":"string"},"description":"Only include results from these domains"},"blocked_domains":{"type":"array","items":{"type":"string"},"description":"Exclude results from these domains"}},"required":["query"]}`),
		}, r.webSearchTool},
		{api.ToolDefinition{
			Name:        "todo_write",
			Description: "Write or update a task list persisted to .glaw/todos.json",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"todos":{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"},"subject":{"type":"string"},"status":{"type":"string"},"description":{"type":"string"}},"required":["id","subject","status"]}}},"required":["todos"]}`),
		}, r.todoWriteTool},
		{api.ToolDefinition{
			Name:        "tool_search",
			Description: "Search available tools by name or description",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Search query to match against tool names and descriptions"}},"required":["query"]}`),
		}, r.toolSearchTool},
		{api.ToolDefinition{
			Name:        "notebook_edit",
			Description: "Edit a Jupyter notebook (.ipynb) file by replacing, inserting, or deleting cells",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"notebook_path":{"type":"string","description":"Absolute or relative path to the .ipynb file"},"cell_id":{"type":"string","description":"ID of the cell to edit or insert after"},"cell_type":{"type":"string","enum":["code","markdown"],"description":"Type of cell for insert"},"new_source":{"type":"string","description":"New source content for the cell"},"edit_mode":{"type":"string","enum":["replace","insert","delete"],"description":"Edit mode: replace, insert, or delete"}},"required":["notebook_path","edit_mode"]}`),
		}, r.notebookEditTool},
		{api.ToolDefinition{
			Name:        "sleep",
			Description: "Delay execution for a specified number of seconds",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"seconds":{"type":"integer","description":"Number of seconds to sleep","minimum":1}},"required":["seconds"]}`),
		}, r.sleepTool},
		{api.ToolDefinition{
			Name:        "send_user_message",
			Description: "Display a message to the user in the response",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"message":{"type":"string","description":"Message to display to the user"}},"required":["message"]}`),
		}, r.sendUserMessageTool},
		{api.ToolDefinition{
			Name:        "config",
			Description: "Read or write configuration settings",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"action":{"type":"string","enum":["read","write"],"description":"Whether to read or write the setting"},"path":{"type":"string","description":"Dot-separated path to the setting (e.g. permissions.mode)"},"value":{"description":"Value to write (any JSON type)"}},"required":["action","path"]}`),
		}, r.configTool},
			{api.ToolDefinition{
				Name:        "analyze",
				Description: "Analyze the project source code to produce a comprehensive summary, dependency graph, and code statistics. Results are saved to .glaw/analysis.json for quick retrieval later.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"mode":{"type":"string","enum":["full","summary","graph"],"description":"Analysis mode: full (complete analysis), summary (quick overview), graph (dependency graph only)","default":"full"},"format":{"type":"string","enum":["text","mermaid","dot","json"],"description":"Output format for dependency graph: text, mermaid, dot, or json","default":"text"}},"required":[]}`),
			}, r.analyzeTool},
		{api.ToolDefinition{
				Name:        "lsp_go_to_definition",
				Description: "Find where a symbol is defined using LSP",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"file_path":{"type":"string","description":"Absolute path to the file"},"line":{"type":"integer","description":"0-indexed line number"},"character":{"type":"integer","description":"0-indexed character offset"}},"required":["file_path","line","character"]}`),
			}, r.lspGoToDefinitionTool},
			{api.ToolDefinition{
				Name:        "lsp_find_references",
				Description: "Find all references to a symbol using LSP",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"file_path":{"type":"string","description":"Absolute path to the file"},"line":{"type":"integer","description":"0-indexed line number"},"character":{"type":"integer","description":"0-indexed character offset"}},"required":["file_path","line","character"]}`),
			}, r.lspFindReferencesTool},
			{api.ToolDefinition{
				Name:        "lsp_hover",
				Description: "Get hover information for a symbol using LSP",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"file_path":{"type":"string","description":"Absolute path to the file"},"line":{"type":"integer","description":"0-indexed line number"},"character":{"type":"integer","description":"0-indexed character offset"}},"required":["file_path","line","character"]}`),
			}, r.lspHoverTool},
			{api.ToolDefinition{
				Name:        "lsp_document_symbol",
				Description: "List symbols in a document using LSP",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"file_path":{"type":"string","description":"Absolute path to the file"}},"required":["file_path"]}`),
			}, r.lspDocumentSymbolTool},
			{api.ToolDefinition{
				Name:        "lsp_workspace_symbol",
				Description: "Search for symbols across the workspace using LSP",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Search query for symbol names"}},"required":["query"]}`),
			}, r.lspWorkspaceSymbolTool},
			{api.ToolDefinition{
				Name:        "lsp_go_to_implementation",
				Description: "Find implementations of a symbol using LSP",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"file_path":{"type":"string","description":"Absolute path to the file"},"line":{"type":"integer","description":"0-indexed line number"},"character":{"type":"integer","description":"0-indexed character offset"}},"required":["file_path","line","character"]}`),
			}, r.lspGoToImplementationTool},
			{api.ToolDefinition{
				Name:        "lsp_incoming_calls",
				Description: "Find callers of a symbol using LSP call hierarchy",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"file_path":{"type":"string","description":"Absolute path to the file"},"line":{"type":"integer","description":"0-indexed line number"},"character":{"type":"integer","description":"0-indexed character offset"}},"required":["file_path","line","character"]}`),
			}, r.lspIncomingCallsTool},
			{api.ToolDefinition{
				Name:        "lsp_outgoing_calls",
				Description: "Find callees of a symbol using LSP call hierarchy",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"file_path":{"type":"string","description":"Absolute path to the file"},"line":{"type":"integer","description":"0-indexed line number"},"character":{"type":"integer","description":"0-indexed character offset"}},"required":["file_path","line","character"]}`),
			}, r.lspOutgoingCallsTool},
		}

		for _, t := range tools {
			r.handlers[t.spec.Name] = t.handler
			r.specs = append(r.specs, t.spec)
		}
	}

// GetToolSpecs returns all registered tool definitions.
func (r *Registry) GetToolSpecs() []api.ToolDefinition {
	return r.specs
}

// ExecuteTool dispatches to the appropriate tool handler.
func (r *Registry) ExecuteTool(ctx context.Context, name string, input json.RawMessage) (*runtime.ToolOutput, error) {
	handler, ok := r.handlers[name]
	if !ok {
		return &runtime.ToolOutput{
			Content: fmt.Sprintf("Unknown tool: %q", name),
			IsError: true,
		}, nil
	}
	return handler(ctx, input)
}

// resolvePath resolves a path relative to the workspace root.
func (r *Registry) resolvePath(p string) (string, error) {
	if filepath.IsAbs(p) {
		abs, err := filepath.Abs(filepath.Clean(p))
		if err != nil {
			return "", err
		}
		return abs, nil
	}
	abs, err := filepath.Abs(filepath.Join(r.workspaceRoot, p))
	if err != nil {
		return "", err
	}
	return abs, nil
}

// --- Tool implementations ---

func (r *Registry) bashTool(ctx context.Context, input json.RawMessage) (*runtime.ToolOutput, error) {
	var args struct {
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return &runtime.ToolOutput{Content: "Invalid input: " + err.Error(), IsError: true}, nil
	}
	if args.Command == "" {
		return &runtime.ToolOutput{Content: "command is required", IsError: true}, nil
	}

	timeout := time.Duration(args.Timeout) * time.Millisecond
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", args.Command)
	cmd.Dir = r.workspaceRoot

	// Check if the command likely needs interactive terminal access
	// (password prompts, sudo, etc.) — these need direct terminal I/O.
	// For most commands, capture output as before.
	needsTerminal := isInteractiveCommand(args.Command)

	if needsTerminal {
		// Connect directly to terminal for interactive commands
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		// Clear the spinner line before running the command
		fmt.Print("\r\033[K")

		err := cmd.Run()
		fmt.Println() // separate from next spinner

		if ctx.Err() == context.DeadlineExceeded {
			return &runtime.ToolOutput{
				Content: fmt.Sprintf("Command timed out after %v", timeout),
				IsError: true,
			}, nil
		}
		if err != nil {
			return &runtime.ToolOutput{
				Content: fmt.Sprintf("Command failed: %v", err),
				IsError: true,
			}, nil
		}
		return &runtime.ToolOutput{Content: "(command completed)", IsError: false}, nil
	}

	// Standard: capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n" + stderr.String()
	}

	if ctx.Err() == context.DeadlineExceeded {
		return &runtime.ToolOutput{
			Content: fmt.Sprintf("Command timed out after %v\n%s", timeout, output),
			IsError: true,
		}, nil
	}

	if err != nil {
		return &runtime.ToolOutput{
			Content: fmt.Sprintf("Command failed: %v\n%s", err, output),
			IsError: true,
		}, nil
	}

	return &runtime.ToolOutput{Content: output, IsError: false}, nil
}

// isInteractiveCommand checks if a bash command likely needs terminal interaction.
func isInteractiveCommand(cmd string) bool {
	// Commands that commonly need user input
	interactivePatterns := []string{
		"sudo ", "passwd", "ssh ", "scp ", "rsync ",
		"docker login", "docker push", "docker pull",
		"gh auth login", "gh auth setup-git",
		"git push", "git pull", "git clone",
		"npm login", "npm publish",
		"read ", "read -p", "read -s",
		"openssl req", "openssl pkcs12",
		"gpg ", "gpg2 ",
		"keytool ", "security ",
		"su ", "su -",
		"login ",
		"curl -u", // curl with user auth may prompt for password
		"mysql -p", "psql -W", "pg_dump -W",
	}
	cmdLower := strings.ToLower(cmd)
	for _, pattern := range interactivePatterns {
		if strings.Contains(cmdLower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

func (r *Registry) readFileTool(ctx context.Context, input json.RawMessage) (*runtime.ToolOutput, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return &runtime.ToolOutput{Content: "Invalid input: " + err.Error(), IsError: true}, nil
	}

	resolved, err := r.resolvePath(args.Path)
	if err != nil {
		return &runtime.ToolOutput{Content: "Invalid path: " + err.Error(), IsError: true}, nil
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("Failed to read %s: %v", args.Path, err), IsError: true}, nil
	}

	return &runtime.ToolOutput{Content: string(data), IsError: false}, nil
}

func (r *Registry) writeFileTool(ctx context.Context, input json.RawMessage) (*runtime.ToolOutput, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return &runtime.ToolOutput{Content: "Invalid input: " + err.Error(), IsError: true}, nil
	}

	resolved, err := r.resolvePath(args.Path)
	if err != nil {
		return &runtime.ToolOutput{Content: "Invalid path: " + err.Error(), IsError: true}, nil
	}

	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("Failed to create directories: %v", err), IsError: true}, nil
	}

	if err := os.WriteFile(resolved, []byte(args.Content), 0o644); err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("Failed to write %s: %v", args.Path, err), IsError: true}, nil
	}

	return &runtime.ToolOutput{Content: fmt.Sprintf("Wrote %d bytes to %s", len(args.Content), args.Path), IsError: false}, nil
}

func (r *Registry) editFileTool(ctx context.Context, input json.RawMessage) (*runtime.ToolOutput, error) {
	var args struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return &runtime.ToolOutput{Content: "Invalid input: " + err.Error(), IsError: true}, nil
	}

	resolved, err := r.resolvePath(args.Path)
	if err != nil {
		return &runtime.ToolOutput{Content: "Invalid path: " + err.Error(), IsError: true}, nil
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("Failed to read %s: %v", args.Path, err), IsError: true}, nil
	}

	content := string(data)
	count := strings.Count(content, args.OldString)
	if count == 0 {
		return &runtime.ToolOutput{
			Content: fmt.Sprintf("old_string not found in %s", args.Path),
			IsError: true,
		}, nil
	}
	if count > 1 {
		return &runtime.ToolOutput{
			Content: fmt.Sprintf("old_string found %d times in %s; must be unique", count, args.Path),
			IsError: true,
		}, nil
	}

	newContent := strings.Replace(content, args.OldString, args.NewString, 1)
	if err := os.WriteFile(resolved, []byte(newContent), 0o644); err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("Failed to write %s: %v", args.Path, err), IsError: true}, nil
	}

	return &runtime.ToolOutput{
		Content: fmt.Sprintf("Edited %s (replaced 1 occurrence)", args.Path),
		IsError: false,
	}, nil
}

func (r *Registry) globSearchTool(ctx context.Context, input json.RawMessage) (*runtime.ToolOutput, error) {
	var args struct {
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return &runtime.ToolOutput{Content: "Invalid input: " + err.Error(), IsError: true}, nil
	}

	searchDir := r.workspaceRoot
	if !filepath.IsAbs(args.Pattern) {
		args.Pattern = filepath.Join(searchDir, args.Pattern)
	}

	matches, err := filepath.Glob(args.Pattern)
	if err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("Invalid pattern: %v", err), IsError: true}, nil
	}

	if len(matches) == 0 {
		return &runtime.ToolOutput{Content: "No files matched the pattern.", IsError: false}, nil
	}

	return &runtime.ToolOutput{
		Content: strings.Join(matches, "\n"),
		IsError: false,
	}, nil
}

func (r *Registry) grepSearchTool(ctx context.Context, input json.RawMessage) (*runtime.ToolOutput, error) {
	var args struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return &runtime.ToolOutput{Content: "Invalid input: " + err.Error(), IsError: true}, nil
	}

	re, err := regexp.Compile(args.Pattern)
	if err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("Invalid regex: %v", err), IsError: true}, nil
	}

	searchPath := args.Path
	if searchPath == "" {
		searchPath = r.workspaceRoot
	} else {
		searchPath, err = r.resolvePath(searchPath)
		if err != nil {
			return &runtime.ToolOutput{Content: "Invalid path: " + err.Error(), IsError: true}, nil
		}
	}

	var results []string

	info, err := os.Stat(searchPath)
	if err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("Path not found: %v", err), IsError: true}, nil
	}

	if !info.IsDir() {
		matches, err := grepFile(searchPath, re)
		if err != nil {
			return &runtime.ToolOutput{Content: err.Error(), IsError: true}, nil
		}
		results = matches
	} else {
		_ = filepath.WalkDir(searchPath, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			matches, err := grepFile(path, re)
			if err != nil {
				return nil
			}
			results = append(results, matches...)
			return nil
		})
	}

	if len(results) == 0 {
		return &runtime.ToolOutput{Content: "No matches found.", IsError: false}, nil
	}

	return &runtime.ToolOutput{
		Content: strings.Join(results, "\n"),
		IsError: false,
	}, nil
}

func grepFile(path string, re *regexp.Regexp) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var results []string
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if re.MatchString(scanner.Text()) {
			results = append(results, fmt.Sprintf("%s:%d: %s", path, lineNum, scanner.Text()))
		}
	}
	return results, scanner.Err()
}

func (r *Registry) webFetchTool(ctx context.Context, input json.RawMessage) (*runtime.ToolOutput, error) {
	var args struct {
		URL     string `json:"url"`
		Timeout int    `json:"timeout"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return &runtime.ToolOutput{Content: "Invalid input: " + err.Error(), IsError: true}, nil
	}
	if args.URL == "" {
		return &runtime.ToolOutput{Content: "url is required", IsError: true}, nil
	}

	timeout := time.Duration(args.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, args.URL, nil)
	if err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("Invalid URL: %v", err), IsError: true}, nil
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("Request failed: %v", err), IsError: true}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024)) // 10KB limit
	if err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("Read failed: %v", err), IsError: true}, nil
	}

	return &runtime.ToolOutput{Content: string(body), IsError: false}, nil
}

// --- New tool implementations ---

func (r *Registry) webSearchTool(ctx context.Context, input json.RawMessage) (*runtime.ToolOutput, error) {
	var args struct {
		Query          string   `json:"query"`
		AllowedDomains []string `json:"allowed_domains"`
		BlockedDomains []string `json:"blocked_domains"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return &runtime.ToolOutput{Content: "Invalid input: " + err.Error(), IsError: true}, nil
	}
	if args.Query == "" {
		return &runtime.ToolOutput{Content: "query is required", IsError: true}, nil
	}

	searchURL := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(args.Query)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("Invalid search URL: %v", err), IsError: true}, nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; GlawCode/1.0)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("Search request failed: %v", err), IsError: true}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 100*1024)) // 100KB limit
	if err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("Read failed: %v", err), IsError: true}, nil
	}

	results := parseDuckDuckGoHTML(string(body))

	// Apply domain filters.
	allowedSet := make(map[string]bool, len(args.AllowedDomains))
	for _, d := range args.AllowedDomains {
		allowedSet[strings.ToLower(d)] = true
	}
	blockedSet := make(map[string]bool, len(args.BlockedDomains))
	for _, d := range args.BlockedDomains {
		blockedSet[strings.ToLower(d)] = true
	}

	filtered := results[:0]
	for _, r := range results {
		domain := extractDomain(r.URL)
		domain = strings.ToLower(domain)
		if len(allowedSet) > 0 && !allowedSet[domain] {
			continue
		}
		if blockedSet[domain] {
			continue
		}
		filtered = append(filtered, r)
	}
	results = filtered

	if len(results) == 0 {
		return &runtime.ToolOutput{Content: "No search results found.", IsError: false}, nil
	}

	var sb strings.Builder
	for i, r := range results {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(fmt.Sprintf("[%d] %s\n%s\n%s", i+1, r.Title, r.URL, r.Snippet))
	}

	return &runtime.ToolOutput{Content: sb.String(), IsError: false}, nil
}

// searchResult holds a parsed DuckDuckGo result.
type searchResult struct {
	Title   string
	URL     string
	Snippet string
}

// parseDuckDuckGoHTML extracts search results from DuckDuckGo HTML response.
func parseDuckDuckGoHTML(html string) []searchResult {
	var results []searchResult

	// Find all result blocks using a simple state-machine parser.
	// DuckDuckGo HTML uses class="result" divs with:
	//   <a rel="nofollow" class="result__a" href="...">Title</a>
	//   <a class="result__url" href="...">...</a>
	//   <a class="result__snippet" href="...">Snippet</a>

	// Extract result__a links (titles + URLs).
	titleRe := regexp.MustCompile(`class="result__a"[^>]*href="([^"]*)"[^>]*>(.*?)</a>`)
	snippetRe := regexp.MustCompile(`class="result__snippet"[^>]*>(.*?)</a>`)

	titleMatches := titleRe.FindAllStringSubmatch(html, -1)
	snippetMatches := snippetRe.FindAllStringSubmatch(html, -1)

	n := len(titleMatches)
	if n == 0 {
		return nil
	}

	for i := 0; i < n; i++ {
		href := unescapeHTML(titleMatches[i][1])
		title := stripTags(titleMatches[i][2])

		snippet := ""
		if i < len(snippetMatches) {
			snippet = stripTags(snippetMatches[i][1])
		}

		// DuckDuckGo uses redirect URLs; extract the actual URL from the uddg parameter.
		actualURL := extractDuckDuckGoURL(href)

		results = append(results, searchResult{
			Title:   strings.TrimSpace(title),
			URL:     actualURL,
			Snippet: strings.TrimSpace(snippet),
		})
	}

	return results
}

// extractDuckDuckGoURL extracts the actual URL from a DuckDuckGo redirect link.
func extractDuckDuckGoURL(href string) string {
	if u, err := url.Parse(href); err == nil {
		if q := u.Query().Get("uddg"); q != "" {
			return q
		}
	}
	return href
}

// stripTags removes HTML tags from a string.
func stripTags(s string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	s = re.ReplaceAllString(s, "")
	return unescapeHTML(s)
}

// unescapeHTML unescapes common HTML entities.
func unescapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&#39;", "'")
	return s
}

// extractDomain returns the hostname from a URL string.
func extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.Hostname()
}

func (r *Registry) todoWriteTool(ctx context.Context, input json.RawMessage) (*runtime.ToolOutput, error) {
	var args struct {
		Todos []struct {
			ID          string `json:"id"`
			Subject     string `json:"subject"`
			Status      string `json:"status"`
			Description string `json:"description"`
		} `json:"todos"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return &runtime.ToolOutput{Content: "Invalid input: " + err.Error(), IsError: true}, nil
	}
	if len(args.Todos) == 0 {
		return &runtime.ToolOutput{Content: "todos array must not be empty", IsError: true}, nil
	}

	// Ensure the .glaw directory exists.
	glawDir := filepath.Join(r.workspaceRoot, ".glaw")
	if err := os.MkdirAll(glawDir, 0o755); err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("Failed to create .glaw directory: %v", err), IsError: true}, nil
	}

	data, err := json.MarshalIndent(args.Todos, "", "  ")
	if err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("Failed to marshal todos: %v", err), IsError: true}, nil
	}

	todoPath := filepath.Join(glawDir, "todos.json")
	if err := os.WriteFile(todoPath, data, 0o644); err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("Failed to write todos: %v", err), IsError: true}, nil
	}

	return &runtime.ToolOutput{
		Content: fmt.Sprintf("Wrote %d todos to .glaw/todos.json", len(args.Todos)),
		IsError: false,
	}, nil
}

func (r *Registry) toolSearchTool(ctx context.Context, input json.RawMessage) (*runtime.ToolOutput, error) {
	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return &runtime.ToolOutput{Content: "Invalid input: " + err.Error(), IsError: true}, nil
	}
	if args.Query == "" {
		return &runtime.ToolOutput{Content: "query is required", IsError: true}, nil
	}

	queryLower := strings.ToLower(args.Query)

	type match struct {
		name   string
		reason string
	}
	var matches []match

	for _, spec := range r.specs {
		if strings.Contains(strings.ToLower(spec.Name), queryLower) {
			matches = append(matches, match{name: spec.Name, reason: "name match"})
		} else if strings.Contains(strings.ToLower(spec.Description), queryLower) {
			matches = append(matches, match{name: spec.Name, reason: "description match"})
		}
	}

	if len(matches) == 0 {
		return &runtime.ToolOutput{Content: "No tools matched the query.", IsError: false}, nil
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].name < matches[j].name
	})

	var sb strings.Builder
	for _, m := range matches {
		sb.WriteString(fmt.Sprintf("- %s (%s)\n", m.name, m.reason))
	}

	return &runtime.ToolOutput{Content: strings.TrimSpace(sb.String()), IsError: false}, nil
}

func (r *Registry) analyzeTool(ctx context.Context, input json.RawMessage) (*runtime.ToolOutput, error) {
	var args struct {
		Mode   string `json:"mode"`
		Format string `json:"format"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return &runtime.ToolOutput{Content: "Invalid input: " + err.Error(), IsError: true}, nil
	}
	if args.Mode == "" {
		args.Mode = "full"
	}
	if args.Format == "" {
		args.Format = "text"
	}

	glawDir := filepath.Join(r.workspaceRoot, ".glaw")
	analysisPath := filepath.Join(glawDir, "analysis.json")

	switch args.Mode {
	case "summary":
		cached, err := loadAnalysis(analysisPath)
		if err == nil && cached != nil {
			return &runtime.ToolOutput{
				Content: fmt.Sprintf("Project Summary (cached from %s):\n\n%s", cached.Timestamp, cached.FormatSummary()),
				IsError: false,
			}, nil
		}
		result := r.performAnalysis()
		if result == nil {
			return &runtime.ToolOutput{Content: "Analysis produced no results.", IsError: true}, nil
		}
		return &runtime.ToolOutput{Content: result.FormatSummary(), IsError: false}, nil

	case "graph":
		result := r.performAnalysis()
		if result == nil {
			return &runtime.ToolOutput{Content: "Analysis produced no results.", IsError: true}, nil
		}
		if err := result.Save(analysisPath); err != nil {
			return &runtime.ToolOutput{Content: fmt.Sprintf("Failed to save analysis: %v", err), IsError: true}, nil
		}
		return &runtime.ToolOutput{Content: result.FormatGraph(args.Format), IsError: false}, nil

	case "full", "":
		result := r.performAnalysis()
		if result == nil {
			return &runtime.ToolOutput{Content: "Analysis produced no results.", IsError: true}, nil
		}
		if err := result.Save(analysisPath); err != nil {
			// still return result, just note cache failure
		}
		output := result.FormatSummary()
		if args.Format == "text" {
			output += "── Dependency Graph (Mermaid) ─────────────\n```mermaid\n" + result.Graph.Mermaid + "```\n"
		} else {
			output += "\n" + result.FormatGraph(args.Format)
		}
		output += fmt.Sprintf("\nAnalysis saved to .glaw/analysis.json\n")
		return &runtime.ToolOutput{Content: output, IsError: false}, nil

	default:
		return &runtime.ToolOutput{
			Content: fmt.Sprintf("Invalid mode %q; must be full, summary, or graph", args.Mode),
			IsError: true,
		}, nil
	}
}

// ---------- Language-agnostic analysis types ----------

// analysisResult holds the complete analysis of a project.
type analysisResult struct {
	Timestamp    string             `json:"timestamp"`
	RootPath     string             `json:"root_path"`
	Summary      analysisSummary    `json:"summary"`
	Modules      []moduleInfo       `json:"modules"`
	Dependencies []dependency       `json:"dependencies"`
	FileTypes    []fileTypeStat     `json:"file_types"`
	Graph        analysisGraph      `json:"graph"`
}

type analysisSummary struct {
	ProjectName         string   `json:"project_name"`
	PrimaryLanguage     string   `json:"primary_language"`
	Languages           []string `json:"languages"`
	TotalFiles          int      `json:"total_files"`
	TotalDirs           int      `json:"total_dirs"`
	TotalLines          int      `json:"total_lines"`
	TotalCodeLines      int      `json:"total_code_lines"`
	TotalCommentLines   int      `json:"total_comment_lines"`
	TotalBlankLines     int      `json:"total_blank_lines"`
	SourceFiles         int      `json:"source_files"`
	TestFiles           int      `json:"test_files"`
	DocFiles            int      `json:"doc_files"`
	ConfigFiles         int      `json:"config_files"`
	TopLevelDirs        []string `json:"top_level_dirs"`
	HasGoMod            bool     `json:"has_go_mod"`
	HasPackageJSON      bool     `json:"has_package_json"`
	HasPipRequirements  bool     `json:"has_pip_requirements"`
	HasPyprojectToml    bool     `json:"has_pyproject_toml"`
	HasCargoToml        bool     `json:"has_cargo_toml"`
	HasPomXml           bool     `json:"has_pom_xml"`
	HasBuildGradle      bool     `json:"has_build_gradle"`
	HasGemfile          bool     `json:"has_gemfile"`
	HasCSProj           bool     `json:"has_csproj"`
	HasDockerfile       bool     `json:"has_dockerfile"`
	HasMakefile         bool     `json:"has_makefile"`
	HasCI               bool     `json:"has_ci"`
	ModuleCount         int      `json:"module_count"`
	EstimatedComplexity string   `json:"estimated_complexity"`
}

type moduleInfo struct {
	Path          string   `json:"path"`
	Language      string   `json:"language"`
	Files         []string `json:"files"`
	Imports       []string `json:"imports"`
	LineCount     int      `json:"line_count"`
	HasTests      bool     `json:"has_tests"`
	FunctionCount int      `json:"function_count"`
}

type dependency struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Type    string `json:"type"` // "internal", "external", "standard"
	Details string `json:"details,omitempty"`
}

type fileTypeStat struct {
	Extension string  `json:"extension"`
	Count     int     `json:"count"`
	Lines     int     `json:"lines"`
	Percentage float64 `json:"percentage"`
}

type analysisGraph struct {
	Mermaid   string              `json:"mermaid"`
	Adjacency map[string][]string `json:"adjacency"`
	DOT       string              `json:"dot"`
}

// ---------- Analysis helpers ----------

// langRules defines per-language rules for import scanning, test detection, etc.
var langRules = map[string]struct {
	Extensions      []string
	TestPatterns    []string
	CommentLine     []string
	CommentBlockO   []string // block comment open
	CommentBlockC   []string // block comment close
	ImportRegex     string
	ModuleFile      string
	ProjectFile     string // top-level project manifest
}{
	"go": {
		Extensions:   []string{".go"},
		TestPatterns: []string{"_test.go"},
		CommentLine:  []string{"//"},
		CommentBlockO: []string{"/*"},
		CommentBlockC: []string{"*/"},
		ImportRegex:  `(?m)^\s*"\s*([^"]+)"\s*$`,
		ModuleFile:   "go.mod",
	},
	"python": {
		Extensions:   []string{".py", ".pyw"},
		TestPatterns: []string{"test_", "test_", "_test.py"},
		CommentLine:  []string{"#"},
		ImportRegex:  `(?m)^\s*(?:from|import)\s+([a-zA-Z_][\w.]*)`,
		ProjectFile:  "pyproject.toml",
	},
	"javascript": {
		Extensions:   []string{".js", ".mjs", ".cjs"},
		TestPatterns: []string{".test.", ".spec."},
		CommentLine:  []string{"//"},
		CommentBlockO: []string{"/*"},
		CommentBlockC: []string{"*/"},
		ImportRegex:  `(?:require\(|import\s+.*?\s+from\s+|import\s+)['"]([^./][^'"]*)['"]`,
	},
	"typescript": {
		Extensions:   []string{".ts", ".tsx", ".cts", ".mts"},
		TestPatterns: []string{".test.", ".spec."},
		CommentLine:  []string{"//"},
		CommentBlockO: []string{"/*"},
		CommentBlockC: []string{"*/"},
		ImportRegex:  `(?:require\(|import\s+.*?\s+from\s+|import\s+)['"]([^./][^'"]*)['"]`,
	},
	"rust": {
		Extensions:   []string{".rs"},
		TestPatterns: []string{},
		CommentLine:  []string{"//", "///", "//!"},
		CommentBlockO: []string{"/*", "/*!"},
		CommentBlockC: []string{"*/"},
		ImportRegex:  `(?m)^\s*(?:use|pub\s+use)\s+([^;{]+)`,
		ModuleFile:   "Cargo.toml",
	},
	"java": {
		Extensions:   []string{".java"},
		TestPatterns: []string{"Test.java", "Tests.java", "IT.java"},
		CommentLine:  []string{"//"},
		CommentBlockO: []string{"/*", "/**"},
		CommentBlockC: []string{"*/"},
		ImportRegex:  `(?m)^\s*import\s+(?:static\s+)?([^;]+)`,
	},
	"ruby": {
		Extensions:   []string{".rb", ".rake"},
		TestPatterns: []string{"_test.rb", "_spec.rb"},
		CommentLine:  []string{"#"},
		ImportRegex:  `(?m)^\s*(?:require|require_relative|gem)\s+['"]([^'"]+)['"]`,
	},
	"csharp": {
		Extensions:   []string{".cs"},
		TestPatterns: []string{"Test.cs", "Tests.cs"},
		CommentLine:  []string{"//"},
		CommentBlockO: []string{"/*"},
		CommentBlockC: []string{"*/"},
		ImportRegex:  `(?m)^\s*using\s+([^;]+)`,
	},
	"cpp": {
		Extensions:   []string{".cpp", ".cc", ".cxx", ".c", ".h", ".hpp", ".hxx"},
		TestPatterns: []string{"_test.", "test_"},
		CommentLine:  []string{"//"},
		CommentBlockO: []string{"/*"},
		CommentBlockC: []string{"*/"},
		ImportRegex:  `(?m)^\s*#\s*include\s*[<"]([^>"]+)[>"]`,
	},
	"swift": {
		Extensions:   []string{".swift"},
		TestPatterns: []string{"Tests.swift", "Test.swift"},
		CommentLine:  []string{"//"},
		CommentBlockO: []string{"/*"},
		CommentBlockC: []string{"*/"},
		ImportRegex:  `(?m)^\s*import\s+(\w+)`,
	},
	"kotlin": {
		Extensions:   []string{".kt", ".kts"},
		TestPatterns: []string{"Test.kt", "Tests.kt"},
		CommentLine:  []string{"//"},
		CommentBlockO: []string{"/*"},
		CommentBlockC: []string{"*/"},
		ImportRegex:  `(?m)^\s*import\s+([^;]+)`,
	},
	"php": {
		Extensions:   []string{".php"},
		TestPatterns: []string{"Test.php", "Tests.php"},
		CommentLine:  []string{"//", "#"},
		CommentBlockO: []string{"/*"},
		CommentBlockC: []string{"*/"},
		ImportRegex:  `(?m)^\s*(?:use|require|require_once|include|include_once)\s+[^;]*['"]([^'"]+)['"]`,
	},
	"scala": {
		Extensions:   []string{".scala"},
		TestPatterns: []string{"Spec.scala", "Test.scala"},
		CommentLine:  []string{"//"},
		CommentBlockO: []string{"/*"},
		CommentBlockC: []string{"*/"},
		ImportRegex:  `(?m)^\s*import\s+([^;{]+)`,
	},
}

var analysisExcludeDirs = []string{
	".git", "node_modules", "vendor", "dist", "build", "target",
	"__pycache__", ".next", ".turbo", "bin", "obj", "out",
	".glaw", ".cache", ".vscode", ".idea", ".tox", ".mypy_cache",
	"venv", ".venv", "env", ".env", ".eggs", "eggs",
	"coverage", ".coverage", ".pytest_cache", ".sass-cache",
	"Pods", ".gradle", ".dart_tool",
}

var analysisExcludeFiles = []string{
	"go.sum", "package-lock.json", "yarn.lock", "pnpm-lock.yaml",
	"Gemfile.lock", "composer.lock", "Cargo.lock", "poetry.lock",
}

// extToLang maps file extensions to language names.
func extToLang(ext string) string {
	for lang, rules := range langRules {
		for _, e := range rules.Extensions {
			if ext == e {
				return lang
			}
		}
	}
	return ""
}

// isAnalysisExcludedDir checks if a directory name should be excluded.
func isAnalysisExcludedDir(name string) bool {
	for _, d := range analysisExcludeDirs {
		if name == d {
			return true
		}
	}
	return false
}

// isAnalysisExcludedFile checks if a file name should be excluded.
func isAnalysisExcludedFile(name string) bool {
	for _, f := range analysisExcludeFiles {
		if name == f {
			return true
		}
	}
	return false
}

// isAnalyzeTextFile returns true for extensions we count lines for.
func isAnalyzeTextFile(ext string) bool {
	for _, rules := range langRules {
		for _, e := range rules.Extensions {
			if ext == e {
				return true
			}
		}
	}
	textExts := map[string]bool{
		".json": true, ".yaml": true, ".yml": true, ".toml": true, ".xml": true,
		".ini": true, ".cfg": true, ".conf": true, ".env": true,
		".md": true, ".txt": true, ".rst": true, ".adoc": true,
		".mod": true, ".sql": true, ".graphql": true, ".proto": true,
		".html": true, ".css": true, ".scss": true, ".less": true,
		".vue": true, ".svelte": true,
		".sh": true, ".bash": true, ".zsh": true, ".fish": true,
		"Makefile": true, "Dockerfile": true,
		".gitignore": true, ".dockerignore": true, ".editorconfig": true,
	}
	return textExts[ext]
}

// isTestFile checks if a file name matches any language's test pattern.
func isTestFile(name string) bool {
	for _, rules := range langRules {
		for _, pat := range rules.TestPatterns {
			if strings.Contains(name, pat) {
				return true
			}
		}
	}
	return false
}

// isSourceFile checks if a file extension belongs to any known language.
func isSourceFile(ext string) bool {
	return extToLang(ext) != ""
}

// isDocFile checks doc extensions.
func isDocFile(ext string) bool {
	return ext == ".md" || ext == ".txt" || ext == ".rst" || ext == ".adoc"
}

// isConfigFile checks config extensions.
func isConfigFile(ext string) bool {
	configs := map[string]bool{
		".json": true, ".yaml": true, ".yml": true, ".toml": true,
		".xml": true, ".ini": true, ".cfg": true, ".conf": true,
		".env": true, ".mod": true, ".lock": true,
		".gitignore": true, ".dockerignore": true, ".editorconfig": true,
	}
	return configs[ext]
}

// analyzeCountLines counts total/code/comment/blank lines with language-aware comment detection.
func analyzeCountLines(path string, ext string) (total, code, comment, blank int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, 0, 0
	}
	lines := strings.Split(string(data), "\n")

	lang := extToLang(ext)
	var lineCommentStarts []string
	var blockOpen, blockClose []string
	if lang != "" {
		rules := langRules[lang]
		lineCommentStarts = rules.CommentLine
		blockOpen = rules.CommentBlockO
		blockClose = rules.CommentBlockC
	}
	// Default fallback
	if len(lineCommentStarts) == 0 && lang == "" {
		lineCommentStarts = []string{"#", "//"}
	}

	inBlock := false
	for _, line := range lines {
		total++
		t := strings.TrimSpace(line)
		if t == "" {
			blank++
			continue
		}
		if inBlock {
			comment++
			for _, c := range blockClose {
				if strings.Contains(t, c) {
					inBlock = false
					break
				}
			}
			continue
		}
		// Check block comment open
		for _, bo := range blockOpen {
			if strings.HasPrefix(t, bo) {
				comment++
				matched := false
				for _, bc := range blockClose {
					if strings.Contains(t[len(bo):], bc) {
						matched = true
						break
					}
				}
				if !matched {
					inBlock = true
				}
				goto nextLine
			}
		}
		// Check line comment
		for _, lc := range lineCommentStarts {
			if strings.HasPrefix(t, lc) {
				comment++
				goto nextLine
			}
		}
		// <!-- for HTML/XML
		if strings.HasPrefix(t, "<!--") {
			comment++
			if !strings.Contains(t[4:], "-->") {
				inBlock = true
			}
			continue
		}
		code++
		continue
	nextLine:
	}
	return total, code, comment, blank
}

// countFunctions approximates function/method count via regex per language.
func countFunctions(data string, lang string) int {
	var re *regexp.Regexp
	switch lang {
	case "go":
		re = regexp.MustCompile(`(?m)^\s*func\s+`)
	case "python":
		re = regexp.MustCompile(`(?m)^\s*def\s+`)
	case "javascript", "typescript":
		re = regexp.MustCompile(`(?m)(?:function\s+\w+|(?:const|let|var)\s+\w+\s*=\s*(?:async\s+)?(?:\([^)]*\)|[\w]*)\s*=>|(?:async\s+)?\w+\s*\([^)]*\)\s*\{)`)
	case "rust":
		re = regexp.MustCompile(`(?m)^\s*(?:pub\s+)?(?:async\s+)?fn\s+`)
	case "java", "kotlin", "scala":
		re = regexp.MustCompile(`(?m)^\s*(?:public|private|protected|static|final|abstract|override|open|suspend|inline)*\s*(?:\w+\s+)+(\w+)\s*\(`)
	case "ruby":
		re = regexp.MustCompile(`(?m)^\s*def\s+`)
	case "csharp":
		re = regexp.MustCompile(`(?m)^\s*(?:public|private|protected|internal|static|virtual|override|async)*\s*(?:\w+\s+)+(\w+)\s*\(`)
	case "cpp":
		re = regexp.MustCompile(`(?m)(?:\w+[\s*&]+)+(\w+)\s*\([^)]*\)\s*\{`)
	case "swift":
		re = regexp.MustCompile(`(?m)^\s*(?:public|private|internal|open|static|override\s+)?func\s+`)
	case "php":
		re = regexp.MustCompile(`(?m)^\s*(?:public|private|protected|static)?\s*function\s+`)
	default:
		return 0
	}
	if re == nil {
		return 0
	}
	return len(re.FindAllString(data, -1))
}

// extractImports scans a file and returns external imports/dependencies.
func extractImports(data string, lang string) []string {
	rules, ok := langRules[lang]
	if !ok || rules.ImportRegex == "" {
		return nil
	}
	re := regexp.MustCompile(rules.ImportRegex)
	matches := re.FindAllStringSubmatch(data, -1)
	var imports []string
	for _, m := range matches {
		if len(m) > 1 {
			imp := strings.TrimSpace(m[1])
			if imp != "" {
				imports = append(imports, imp)
			}
		}
	}
	return imports
}

// scanImportsInFile reads a file and extracts imports using language rules.
func scanImportsInFile(path string, lang string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return extractImports(string(data), lang)
}

// performAnalysis runs a full language-agnostic analysis on the workspace.
func (r *Registry) performAnalysis() *analysisResult {
	result := &analysisResult{
		Timestamp: time.Now().Format(time.RFC3339),
		RootPath:  r.workspaceRoot,
	}
	s := &result.Summary
	s.ProjectName = filepath.Base(r.workspaceRoot)
	s.TopLevelDirs = []string{}
	s.Languages = []string{}

	// Detect infrastructure
	s.HasGoMod = analysisFileExists(filepath.Join(r.workspaceRoot, "go.mod"))
	s.HasPackageJSON = analysisFileExists(filepath.Join(r.workspaceRoot, "package.json"))
	s.HasPipRequirements = analysisFileExists(filepath.Join(r.workspaceRoot, "requirements.txt"))
	s.HasPyprojectToml = analysisFileExists(filepath.Join(r.workspaceRoot, "pyproject.toml"))
	s.HasCargoToml = analysisFileExists(filepath.Join(r.workspaceRoot, "Cargo.toml"))
	s.HasPomXml = analysisFileExists(filepath.Join(r.workspaceRoot, "pom.xml"))
	s.HasBuildGradle = analysisFileExists(filepath.Join(r.workspaceRoot, "build.gradle")) ||
		analysisFileExists(filepath.Join(r.workspaceRoot, "build.gradle.kts"))
	s.HasGemfile = analysisFileExists(filepath.Join(r.workspaceRoot, "Gemfile"))
	s.HasCSProj = analysisGlobExists(r.workspaceRoot, "*.csproj")
	s.HasDockerfile = analysisFileExists(filepath.Join(r.workspaceRoot, "Dockerfile")) ||
		analysisGlobExists(r.workspaceRoot, "Dockerfile.*")
	s.HasMakefile = analysisFileExists(filepath.Join(r.workspaceRoot, "Makefile"))
	s.HasCI = analysisFileExists(filepath.Join(r.workspaceRoot, ".github", "workflows")) ||
		analysisFileExists(filepath.Join(r.workspaceRoot, ".gitlab-ci.yml")) ||
		analysisFileExists(filepath.Join(r.workspaceRoot, "Jenkinsfile"))

	// Walk directory tree
	extMap := make(map[string]*fileTypeStat)
	dirSet := make(map[string]bool)
	// moduleMap: dir -> moduleInfo (group files by directory per language)
	moduleMap := make(map[string]*moduleInfo)

	_ = filepath.WalkDir(r.workspaceRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		relPath, relErr := filepath.Rel(r.workspaceRoot, path)
		if relErr != nil {
			return nil
		}

		if d.IsDir() {
			if relPath != "." && isAnalysisExcludedDir(filepath.Base(path)) {
				return filepath.SkipDir
			}
			if relPath != "." {
				s.TotalDirs++
				parts := strings.Split(relPath, string(filepath.Separator))
				dirSet[parts[0]] = true
			}
			return nil
		}

		if isAnalysisExcludedFile(filepath.Base(path)) {
			return nil
		}

		s.TotalFiles++
		ext := strings.ToLower(filepath.Ext(path))
		if ext == "" {
			base := filepath.Base(path)
			switch base {
			case "Makefile", "Dockerfile", "Jenkinsfile", "Vagrantfile":
				ext = base
			default:
				ext = "other"
			}
		}

		ft, ok := extMap[ext]
		if !ok {
			ft = &fileTypeStat{Extension: ext}
			extMap[ext] = ft
		}
		ft.Count++

		// Count lines
		if isAnalyzeTextFile(ext) {
			total, code, comment, blank := analyzeCountLines(path, ext)
			ft.Lines += total
			s.TotalLines += total
			s.TotalCodeLines += code
			s.TotalCommentLines += comment
			s.TotalBlankLines += blank
		}

		// Classify
		base := filepath.Base(path)
		lang := extToLang(ext)
		switch {
		case lang != "" && isTestFile(base):
			s.TestFiles++
			s.SourceFiles++
		case lang != "":
			s.SourceFiles++
		case isDocFile(ext):
			s.DocFiles++
		case isConfigFile(ext):
			s.ConfigFiles++
		}

		// Build per-directory modules for source files
		if lang != "" {
			dir := filepath.Dir(path)
			relDir, _ := filepath.Rel(r.workspaceRoot, dir)
			if relDir == "." {
				relDir = "(root)"
			}
			key := lang + "::" + relDir
			mod, ok := moduleMap[key]
			if !ok {
				mod = &moduleInfo{
					Path:     relDir,
					Language: lang,
					Files:    []string{},
					Imports:  []string{},
				}
				moduleMap[key] = mod
			}
			mod.Files = append(mod.Files, filepath.Base(path))
			if isTestFile(base) {
				mod.HasTests = true
			}
			if total, _, _, _ := analyzeCountLines(path, ext); total > 0 {
				mod.LineCount += total
			}
			// Scan imports
			imports := scanImportsInFile(path, lang)
			for _, imp := range imports {
				found := false
				for _, existing := range mod.Imports {
					if existing == imp {
						found = true
						break
					}
				}
				if !found {
					mod.Imports = append(mod.Imports, imp)
				}
			}
			// Count functions
			if data, err := os.ReadFile(path); err == nil {
				mod.FunctionCount += countFunctions(string(data), lang)
			}
		}
		return nil
	})

	// Convert ext map
	for _, ft := range extMap {
		result.FileTypes = append(result.FileTypes, *ft)
	}
	sort.Slice(result.FileTypes, func(i, j int) bool {
		return result.FileTypes[i].Count > result.FileTypes[j].Count
	})
	totalF := s.TotalFiles
	if totalF == 0 {
		totalF = 1
	}
	for i := range result.FileTypes {
		result.FileTypes[i].Percentage = float64(result.FileTypes[i].Count) / float64(totalF) * 100
	}

	// Top-level dirs
	for dir := range dirSet {
		s.TopLevelDirs = append(s.TopLevelDirs, dir)
	}
	sort.Strings(s.TopLevelDirs)

	// Detect languages
	langCounts := make(map[string]int)
	for ext, ft := range extMap {
		lang := extToLang(ext)
		if lang != "" {
			langCounts[lang] += ft.Count
		}
	}
	type langCount struct{ lang string; count int }
	var lcs []langCount
	for l, c := range langCounts {
		lcs = append(lcs, langCount{l, c})
	}
	sort.Slice(lcs, func(i, j int) bool { return lcs[i].count > lcs[j].count })
	for _, lc := range lcs {
		s.Languages = append(s.Languages, lc.lang)
	}
	if len(lcs) > 0 {
		s.PrimaryLanguage = lcs[0].lang
	}

	// Convert modules
	for _, mod := range moduleMap {
		sort.Strings(mod.Files)
		sort.Strings(mod.Imports)
		result.Modules = append(result.Modules, *mod)
	}
	sort.Slice(result.Modules, func(i, j int) bool {
		return result.Modules[i].Path < result.Modules[i].Path
	})
	s.ModuleCount = len(result.Modules)

	// Build dependency graph
	buildAnalysisGraph(result)

	// Compute complexity
	computeAnalysisComplexity(result)

	return result
}

func buildAnalysisGraph(result *analysisResult) {
	adj := make(map[string][]string)
	importSeen := make(map[string]bool)

	for _, mod := range result.Modules {
		for _, imp := range mod.Imports {
			depType := "external"
			if isStandardLib(imp, mod.Language) {
				depType = "standard"
			}
			// Internal deps: other modules in the same project
			isInternal := false
			for _, other := range result.Modules {
				if other.Path == mod.Path {
					continue
				}
				if strings.Contains(imp, filepath.Base(other.Path)) || strings.HasPrefix(imp, other.Path) {
					isInternal = true
				}
			}
			if isInternal {
				depType = "internal"
			}

			dep := dependency{From: mod.Path, To: imp, Type: depType}
			key := dep.From + "|" + dep.To
			if !importSeen[key] {
				importSeen[key] = true
				result.Dependencies = append(result.Dependencies, dep)
			}

			if depType == "internal" || isInternal {
				from := mermaidSafe(mod.Path)
				to := mermaidSafe(imp)
				adj[from] = append(adj[from], to)
			}
		}
	}

	// Deduplicate
	for k, v := range adj {
		seen := make(map[string]bool)
		unique := v[:0]
		for _, s := range v {
			if !seen[s] {
				seen[s] = true
				unique = append(unique, s)
			}
		}
		sort.Strings(unique)
		adj[k] = unique
	}

	// Mermaid
	var mermaid strings.Builder
	mermaid.WriteString("graph TD\n")
	nodes := make([]string, 0, len(adj))
	for k := range adj {
		nodes = append(nodes, k)
	}
	sort.Strings(nodes)
	for _, node := range nodes {
		for _, dep := range adj[node] {
			mermaid.WriteString(fmt.Sprintf("    %s --> %s\n", node, dep))
		}
	}
	// Add isolated modules
	for _, mod := range result.Modules {
		safe := mermaidSafe(mod.Path)
		if _, ok := adj[safe]; !ok {
			mermaid.WriteString(fmt.Sprintf("    %s\n", safe))
		}
	}

	// DOT
	var dot strings.Builder
	dot.WriteString("digraph project {\n")
	dot.WriteString("    rankdir=TD;\n")
	dot.WriteString("    node [shape=box, style=filled, fillcolor=lightblue];\n")
	for _, node := range nodes {
		for _, dep := range adj[node] {
			dot.WriteString(fmt.Sprintf("    \"%s\" -> \"%s\";\n", node, dep))
		}
	}
	dot.WriteString("}\n")

	result.Graph = analysisGraph{
		Mermaid:   mermaid.String(),
		Adjacency: adj,
		DOT:       dot.String(),
	}
}

// isStandardLib returns true for known standard library prefixes per language.
func isStandardLib(imp string, lang string) bool {
	switch lang {
	case "go":
		stdGo := []string{"fmt", "os", "io", "net", "http", "strings", "strconv", "time",
			"encoding", "context", "sync", "path", "filepath", "bufio", "bytes",
			"errors", "math", "regexp", "sort", "json", "log", "runtime", "reflect",
			"unicode", "crypto", "hash", "testing", "flag", "template", "html",
			"database", "sql", "compress", "archive", "debug", "plugin", "unsafe"}
		for _, prefix := range stdGo {
			if imp == prefix || strings.HasPrefix(imp, prefix+"/") {
				return true
			}
		}
	case "python":
		stdPy := []string{"os", "sys", "json", "re", "time", "datetime", "math",
			"collections", "itertools", "functools", "pathlib", "logging",
			"unittest", "argparse", "subprocess", "threading", "asyncio",
			"dataclasses", "typing", "abc", "io", "csv", "hashlib", "copy",
			"struct", "socket", "http", "urllib", "email", "html", "xml",
			"sqlite3", "random", "string", "shutil", "tempfile", "pickle"}
		parts := strings.Split(imp, ".")
		for _, prefix := range stdPy {
			if parts[0] == prefix {
				return true
			}
		}
	case "rust":
		if imp == "std" || strings.HasPrefix(imp, "std::") {
			return true
		}
	case "java":
		if strings.HasPrefix(imp, "java.") || strings.HasPrefix(imp, "javax.") ||
			strings.HasPrefix(imp, "sun.") {
			return true
		}
	case "csharp":
		if strings.HasPrefix(imp, "System") || strings.HasPrefix(imp, "Microsoft.") {
			return true
		}
	case "ruby":
		stdRb := []string{"json", "csv", "fileutils", "optparse", "logger", "time",
			"date", "uri", "net", "open-uri", "pathname", "tempfile", "socket",
			"digest", "benchmark", "set", "singleton", "forwardable"}
		for _, prefix := range stdRb {
			if imp == prefix || strings.HasPrefix(imp, prefix+"/") {
				return true
			}
		}
	}
	return false
}

func computeAnalysisComplexity(result *analysisResult) {
	s := &result.Summary
	score := 0
	if s.TotalFiles > 50 {
		score += 2
	} else if s.TotalFiles > 20 {
		score += 1
	}
	if s.TotalLines > 10000 {
		score += 2
	} else if s.TotalLines > 3000 {
		score += 1
	}
	if s.ModuleCount > 10 {
		score += 2
	} else if s.ModuleCount > 5 {
		score += 1
	}
	if s.HasDockerfile {
		score++
	}
	if s.HasCI {
		score++
	}
	if len(s.TopLevelDirs) > 8 {
		score++
	}
	switch {
	case score >= 7:
		s.EstimatedComplexity = "large"
	case score >= 4:
		s.EstimatedComplexity = "medium"
	default:
		s.EstimatedComplexity = "small"
	}
}

func analysisFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func analysisGlobExists(dir string, pattern string) bool {
	matches, _ := filepath.Glob(filepath.Join(dir, pattern))
	return len(matches) > 0
}

func mermaidSafe(s string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	return re.ReplaceAllString(s, "_")
}

// ---------- analysisResult formatting ----------

func (r *analysisResult) FormatSummary() string {
	var sb strings.Builder
	s := &r.Summary

	sb.WriteString("═══════════════════════════════════════════\n")
	sb.WriteString("          PROJECT ANALYSIS REPORT          \n")
	sb.WriteString("═══════════════════════════════════════════\n\n")

	sb.WriteString(fmt.Sprintf("Project:      %s\n", s.ProjectName))
	sb.WriteString(fmt.Sprintf("Language:     %s\n", s.PrimaryLanguage))
	if len(s.Languages) > 1 {
		sb.WriteString(fmt.Sprintf("Languages:    %s\n", strings.Join(s.Languages, ", ")))
	}
	sb.WriteString(fmt.Sprintf("Complexity:   %s\n", s.EstimatedComplexity))
	sb.WriteString(fmt.Sprintf("Analyzed:     %s\n", r.Timestamp))
	sb.WriteString("\n")

	sb.WriteString("── Structure ──────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("Total Files:     %d\n", s.TotalFiles))
	sb.WriteString(fmt.Sprintf("  Source Files:  %d\n", s.SourceFiles))
	sb.WriteString(fmt.Sprintf("  Test Files:    %d\n", s.TestFiles))
	sb.WriteString(fmt.Sprintf("  Doc Files:     %d\n", s.DocFiles))
	sb.WriteString(fmt.Sprintf("  Config Files:  %d\n", s.ConfigFiles))
	sb.WriteString(fmt.Sprintf("Directories:     %d\n", s.TotalDirs))
	if s.ModuleCount > 0 {
		sb.WriteString(fmt.Sprintf("Modules:         %d\n", s.ModuleCount))
	}
	sb.WriteString("\n")

	sb.WriteString("── Lines of Code ──────────────────────────\n")
	sb.WriteString(fmt.Sprintf("Total Lines:     %d\n", s.TotalLines))
	sb.WriteString(fmt.Sprintf("  Code:          %d\n", s.TotalCodeLines))
	sb.WriteString(fmt.Sprintf("  Comments:      %d\n", s.TotalCommentLines))
	sb.WriteString(fmt.Sprintf("  Blank:         %d\n", s.TotalBlankLines))
	sb.WriteString("\n")

	sb.WriteString("── Top-Level Directories ──────────────────\n")
	for _, dir := range s.TopLevelDirs {
		sb.WriteString(fmt.Sprintf("  %s/\n", dir))
	}
	sb.WriteString("\n")

	sb.WriteString("── File Type Distribution ─────────────────\n")
	for _, ft := range r.FileTypes {
		if ft.Count > 0 {
			bar := strings.Repeat("█", int(ft.Percentage/5))
			sb.WriteString(fmt.Sprintf("  %-10s %3d files  %s (%.1f%%)\n", ft.Extension, ft.Count, bar, ft.Percentage))
		}
	}
	sb.WriteString("\n")

	sb.WriteString("── Infrastructure ─────────────────────────\n")
	if s.HasGoMod { sb.WriteString("  ✓ go.mod\n") }
	if s.HasPackageJSON { sb.WriteString("  ✓ package.json\n") }
	if s.HasPipRequirements { sb.WriteString("  ✓ requirements.txt\n") }
	if s.HasPyprojectToml { sb.WriteString("  ✓ pyproject.toml\n") }
	if s.HasCargoToml { sb.WriteString("  ✓ Cargo.toml\n") }
	if s.HasPomXml { sb.WriteString("  ✓ pom.xml\n") }
	if s.HasBuildGradle { sb.WriteString("  ✓ build.gradle\n") }
	if s.HasGemfile { sb.WriteString("  ✓ Gemfile\n") }
	if s.HasCSProj { sb.WriteString("  ✓ .csproj\n") }
	if s.HasDockerfile { sb.WriteString("  ✓ Dockerfile\n") }
	if s.HasMakefile { sb.WriteString("  ✓ Makefile\n") }
	if s.HasCI { sb.WriteString("  ✓ CI Config\n") }
	sb.WriteString("\n")

	if len(r.Modules) > 0 {
		sb.WriteString("── Modules ────────────────────────────────\n")
		for _, mod := range r.Modules {
			sb.WriteString(fmt.Sprintf("  %-20s %-12s %d files, %d lines, %d funcs",
				mod.Path, mod.Language, len(mod.Files), mod.LineCount, mod.FunctionCount))
			if mod.HasTests {
				sb.WriteString(" ✓ tested")
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	internalDeps := 0
	externalDeps := 0
	uniqueExternal := make(map[string]bool)
	for _, dep := range r.Dependencies {
		if dep.Type == "internal" {
			internalDeps++
		} else if dep.Type == "external" {
			externalDeps++
			uniqueExternal[dep.To] = true
		}
	}
	sb.WriteString("── Dependencies ───────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  Internal:  %d\n", internalDeps))
	sb.WriteString(fmt.Sprintf("  External:  %d unique\n", len(uniqueExternal)))
	sb.WriteString("\n")

	return sb.String()
}

func (r *analysisResult) FormatGraph(format string) string {
	switch format {
	case "mermaid", "mmd":
		return r.Graph.Mermaid
	case "dot", "graphviz":
		return r.Graph.DOT
	case "adjacency", "json":
		data, _ := json.MarshalIndent(r.Graph.Adjacency, "", "  ")
		return string(data)
	default:
		return r.Graph.Mermaid
	}
}

func (r *analysisResult) Save(path string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling analysis: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

func loadAnalysis(path string) (*analysisResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var result analysisResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *Registry) notebookEditTool(ctx context.Context, input json.RawMessage) (*runtime.ToolOutput, error) {
	var args struct {
		NotebookPath string `json:"notebook_path"`
		CellID       string `json:"cell_id"`
		CellType     string `json:"cell_type"`
		NewSource    string `json:"new_source"`
		EditMode     string `json:"edit_mode"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return &runtime.ToolOutput{Content: "Invalid input: " + err.Error(), IsError: true}, nil
	}
	if args.NotebookPath == "" {
		return &runtime.ToolOutput{Content: "notebook_path is required", IsError: true}, nil
	}
	if args.EditMode == "" {
		return &runtime.ToolOutput{Content: "edit_mode is required", IsError: true}, nil
	}

	resolved, err := r.resolvePath(args.NotebookPath)
	if err != nil {
		return &runtime.ToolOutput{Content: "Invalid path: " + err.Error(), IsError: true}, nil
	}

	// Read the notebook file.
	data, err := os.ReadFile(resolved)
	if err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("Failed to read notebook: %v", err), IsError: true}, nil
	}

	var nb map[string]json.RawMessage
	if err := json.Unmarshal(data, &nb); err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("Invalid notebook format: %v", err), IsError: true}, nil
	}

	cellsRaw, ok := nb["cells"]
	if !ok {
		return &runtime.ToolOutput{Content: "Notebook has no cells array", IsError: true}, nil
	}

	var cells []map[string]interface{}
	if err := json.Unmarshal(cellsRaw, &cells); err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("Invalid cells format: %v", err), IsError: true}, nil
	}

	switch args.EditMode {
	case "replace":
		if args.CellID == "" {
			return &runtime.ToolOutput{Content: "cell_id is required for replace mode", IsError: true}, nil
		}
		idx := findCellByID(cells, args.CellID)
		if idx < 0 {
			return &runtime.ToolOutput{Content: fmt.Sprintf("Cell with id %q not found", args.CellID), IsError: true}, nil
		}
		cells[idx]["source"] = args.NewSource
		if args.CellType != "" {
			cells[idx]["cell_type"] = args.CellType
		}

	case "insert":
		cellType := args.CellType
		if cellType == "" {
			cellType = "code"
		}
		newCell := map[string]interface{}{
			"cell_type": cellType,
			"source":    args.NewSource,
			"id":        args.CellID,
		}
		if cellType == "code" {
			newCell["outputs"] = []interface{}{}
			newCell["execution_count"] = nil
		}
		newCell["metadata"] = map[string]interface{}{}

		if args.CellID != "" {
			idx := findCellByID(cells, args.CellID)
			if idx >= 0 {
				// Insert after the found cell.
				cells = append(cells[:idx+1], append([]map[string]interface{}{newCell}, cells[idx+1:]...)...)
			} else {
				cells = append(cells, newCell)
			}
		} else {
			cells = append(cells, newCell)
		}

	case "delete":
		if args.CellID == "" {
			return &runtime.ToolOutput{Content: "cell_id is required for delete mode", IsError: true}, nil
		}
		idx := findCellByID(cells, args.CellID)
		if idx < 0 {
			return &runtime.ToolOutput{Content: fmt.Sprintf("Cell with id %q not found", args.CellID), IsError: true}, nil
		}
		cells = append(cells[:idx], cells[idx+1:]...)

	default:
		return &runtime.ToolOutput{
			Content: fmt.Sprintf("Invalid edit_mode %q; must be replace, insert, or delete", args.EditMode),
			IsError: true,
		}, nil
	}

	// Write back.
	cellsJSON, err := json.Marshal(cells)
	if err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("Failed to marshal cells: %v", err), IsError: true}, nil
	}
	nb["cells"] = cellsJSON

	nbJSON, err := json.MarshalIndent(nb, "", "  ")
	if err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("Failed to marshal notebook: %v", err), IsError: true}, nil
	}

	if err := os.WriteFile(resolved, nbJSON, 0o644); err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("Failed to write notebook: %v", err), IsError: true}, nil
	}

	return &runtime.ToolOutput{
		Content: fmt.Sprintf("Notebook %s edited (%s mode)", args.NotebookPath, args.EditMode),
		IsError: false,
	}, nil
}

// findCellByID returns the index of the cell with the given id, or -1.
func findCellByID(cells []map[string]interface{}, id string) int {
	for i, c := range cells {
		cid, ok := c["id"]
		if !ok {
			continue
		}
		if s, ok := cid.(string); ok && s == id {
			return i
		}
		// Also check numeric IDs converted to string.
		switch v := cid.(type) {
		case float64:
			if strconv.Itoa(int(v)) == id {
				return i
			}
		}
	}
	return -1
}

func (r *Registry) sleepTool(ctx context.Context, input json.RawMessage) (*runtime.ToolOutput, error) {
	var args struct {
		Seconds int `json:"seconds"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return &runtime.ToolOutput{Content: "Invalid input: " + err.Error(), IsError: true}, nil
	}
	if args.Seconds < 1 {
		return &runtime.ToolOutput{Content: "seconds must be at least 1", IsError: true}, nil
	}

	dur := time.Duration(args.Seconds) * time.Second
	select {
	case <-ctx.Done():
		return &runtime.ToolOutput{Content: "Sleep interrupted: " + ctx.Err().Error(), IsError: true}, nil
	case <-time.After(dur):
		return &runtime.ToolOutput{
			Content: fmt.Sprintf("Slept for %d seconds", args.Seconds),
			IsError: false,
		}, nil
	}
}

func (r *Registry) sendUserMessageTool(ctx context.Context, input json.RawMessage) (*runtime.ToolOutput, error) {
	var args struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return &runtime.ToolOutput{Content: "Invalid input: " + err.Error(), IsError: true}, nil
	}
	if args.Message == "" {
		return &runtime.ToolOutput{Content: "message is required", IsError: true}, nil
	}

	// Return the message as content; the caller displays it in the response.
	return &runtime.ToolOutput{Content: args.Message, IsError: false}, nil
}

func (r *Registry) configTool(ctx context.Context, input json.RawMessage) (*runtime.ToolOutput, error) {
	var args struct {
		Action string          `json:"action"`
		Path   string          `json:"path"`
		Value  json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return &runtime.ToolOutput{Content: "Invalid input: " + err.Error(), IsError: true}, nil
	}
	if args.Action == "" {
		return &runtime.ToolOutput{Content: "action is required", IsError: true}, nil
	}
	if args.Path == "" {
		return &runtime.ToolOutput{Content: "path is required", IsError: true}, nil
	}

	// Load current settings.
	settings, err := config.LoadAll(r.workspaceRoot)
	if err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("Failed to load settings: %v", err), IsError: true}, nil
	}

	switch args.Action {
	case "read":
		val, err := getSettingByPath(&settings, args.Path)
		if err != nil {
			return &runtime.ToolOutput{Content: err.Error(), IsError: true}, nil
		}
		result, _ := json.MarshalIndent(val, "", "  ")
		return &runtime.ToolOutput{Content: string(result), IsError: false}, nil

	case "write":
		if len(args.Value) == 0 {
			return &runtime.ToolOutput{Content: "value is required for write action", IsError: true}, nil
		}
		var val interface{}
		if err := json.Unmarshal(args.Value, &val); err != nil {
			return &runtime.ToolOutput{Content: fmt.Sprintf("Invalid value: %v", err), IsError: true}, nil
		}
		if err := setSettingByPath(&settings, args.Path, val); err != nil {
			return &runtime.ToolOutput{Content: err.Error(), IsError: true}, nil
		}
		// Determine whether to save to global or project config.
		// We save to the project config for workspace-scoped changes.
		if err := config.SaveProject(r.workspaceRoot, settings); err != nil {
			return &runtime.ToolOutput{Content: fmt.Sprintf("Failed to save settings: %v", err), IsError: true}, nil
		}
		return &runtime.ToolOutput{
			Content: fmt.Sprintf("Setting %s updated", args.Path),
			IsError: false,
		}, nil

	default:
		return &runtime.ToolOutput{
			Content: fmt.Sprintf("Invalid action %q; must be read or write", args.Action),
			IsError: true,
		}, nil
	}
}

// getSettingByPath retrieves a nested setting value by dot-separated path.
func getSettingByPath(s *config.Settings, path string) (interface{}, error) {
	parts := strings.Split(path, ".")
	// Marshal and unmarshal the settings to a generic map for dynamic access.
	data, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal settings: %w", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to unmarshal settings: %w", err)
	}

	current := interface{}(m)
	for _, part := range parts {
		asMap, ok := current.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("path %q does not exist (not an object at %q)", path, part)
		}
		current, ok = asMap[part]
		if !ok {
			return nil, fmt.Errorf("path %q does not exist (missing key %q)", path, part)
		}
	}
	return current, nil
}

// setSettingByPath sets a nested setting value by dot-separated path.
func setSettingByPath(s *config.Settings, path string, value interface{}) error {
	parts := strings.Split(path, ".")
	// Marshal and unmarshal the settings to a generic map for dynamic access.
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("failed to unmarshal settings: %w", err)
	}

	current := m
	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			break
		}
		next, ok := current[part]
		if !ok {
			newMap := make(map[string]interface{})
			current[part] = newMap
			current = newMap
			continue
		}
		nextMap, ok := next.(map[string]interface{})
		if !ok {
			return fmt.Errorf("path %q does not exist (not an object at %q)", path, part)
		}
		current = nextMap
	}

	// Marshal back to the Settings struct.
	newData, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("failed to marshal updated settings: %w", err)
	}
	if err := json.Unmarshal(newData, s); err != nil {
		return fmt.Errorf("failed to unmarshal updated settings: %w", err)
	}

	return nil
}

// --- LSP Tool implementations ---

func (r *Registry) lspGoToDefinitionTool(ctx context.Context, input json.RawMessage) (*runtime.ToolOutput, error) {
	if r.lspManager == nil {
		return &runtime.ToolOutput{Content: "LSP is not available. No language server configured.", IsError: true}, nil
	}
	var args struct {
		FilePath string `json:"file_path"`
		Line     int    `json:"line"`
		Character int   `json:"character"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return &runtime.ToolOutput{Content: "Invalid input: " + err.Error(), IsError: true}, nil
	}
	locs, err := r.lspManager.GoToDefinition(ctx, args.FilePath, args.Line, args.Character)
	if err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("LSP error: %v", err), IsError: true}, nil
	}
	if len(locs) == 0 {
		return &runtime.ToolOutput{Content: "No definitions found.", IsError: false}, nil
	}
	return &runtime.ToolOutput{Content: formatSymbolLocations(locs), IsError: false}, nil
}

func (r *Registry) lspFindReferencesTool(ctx context.Context, input json.RawMessage) (*runtime.ToolOutput, error) {
	if r.lspManager == nil {
		return &runtime.ToolOutput{Content: "LSP is not available. No language server configured.", IsError: true}, nil
	}
	var args struct {
		FilePath string `json:"file_path"`
		Line     int    `json:"line"`
		Character int   `json:"character"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return &runtime.ToolOutput{Content: "Invalid input: " + err.Error(), IsError: true}, nil
	}
	locs, err := r.lspManager.FindReferences(ctx, args.FilePath, args.Line, args.Character)
	if err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("LSP error: %v", err), IsError: true}, nil
	}
	if len(locs) == 0 {
		return &runtime.ToolOutput{Content: "No references found.", IsError: false}, nil
	}
	return &runtime.ToolOutput{Content: formatSymbolLocations(locs), IsError: false}, nil
}

func (r *Registry) lspHoverTool(ctx context.Context, input json.RawMessage) (*runtime.ToolOutput, error) {
	if r.lspManager == nil {
		return &runtime.ToolOutput{Content: "LSP is not available. No language server configured.", IsError: true}, nil
	}
	var args struct {
		FilePath string `json:"file_path"`
		Line     int    `json:"line"`
		Character int   `json:"character"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return &runtime.ToolOutput{Content: "Invalid input: " + err.Error(), IsError: true}, nil
	}
	result, err := r.lspManager.Hover(ctx, args.FilePath, args.Line, args.Character)
	if err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("LSP error: %v", err), IsError: true}, nil
	}
	if result == "" {
		return &runtime.ToolOutput{Content: "No hover information available.", IsError: false}, nil
	}
	return &runtime.ToolOutput{Content: result, IsError: false}, nil
}

func (r *Registry) lspDocumentSymbolTool(ctx context.Context, input json.RawMessage) (*runtime.ToolOutput, error) {
	if r.lspManager == nil {
		return &runtime.ToolOutput{Content: "LSP is not available. No language server configured.", IsError: true}, nil
	}
	var args struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return &runtime.ToolOutput{Content: "Invalid input: " + err.Error(), IsError: true}, nil
	}
	locs, err := r.lspManager.DocumentSymbol(ctx, args.FilePath)
	if err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("LSP error: %v", err), IsError: true}, nil
	}
	if len(locs) == 0 {
		return &runtime.ToolOutput{Content: "No symbols found in document.", IsError: false}, nil
	}
	return &runtime.ToolOutput{Content: formatSymbolLocations(locs), IsError: false}, nil
}

func (r *Registry) lspWorkspaceSymbolTool(ctx context.Context, input json.RawMessage) (*runtime.ToolOutput, error) {
	if r.lspManager == nil {
		return &runtime.ToolOutput{Content: "LSP is not available. No language server configured.", IsError: true}, nil
	}
	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return &runtime.ToolOutput{Content: "Invalid input: " + err.Error(), IsError: true}, nil
	}
	locs, err := r.lspManager.WorkspaceSymbol(ctx, args.Query)
	if err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("LSP error: %v", err), IsError: true}, nil
	}
	if len(locs) == 0 {
		return &runtime.ToolOutput{Content: "No workspace symbols found.", IsError: false}, nil
	}
	return &runtime.ToolOutput{Content: formatSymbolLocations(locs), IsError: false}, nil
}

func (r *Registry) lspGoToImplementationTool(ctx context.Context, input json.RawMessage) (*runtime.ToolOutput, error) {
	if r.lspManager == nil {
		return &runtime.ToolOutput{Content: "LSP is not available. No language server configured.", IsError: true}, nil
	}
	var args struct {
		FilePath string `json:"file_path"`
		Line     int    `json:"line"`
		Character int   `json:"character"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return &runtime.ToolOutput{Content: "Invalid input: " + err.Error(), IsError: true}, nil
	}
	locs, err := r.lspManager.GoToImplementation(ctx, args.FilePath, args.Line, args.Character)
	if err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("LSP error: %v", err), IsError: true}, nil
	}
	if len(locs) == 0 {
		return &runtime.ToolOutput{Content: "No implementations found.", IsError: false}, nil
	}
	return &runtime.ToolOutput{Content: formatSymbolLocations(locs), IsError: false}, nil
}

func (r *Registry) lspIncomingCallsTool(ctx context.Context, input json.RawMessage) (*runtime.ToolOutput, error) {
	if r.lspManager == nil {
		return &runtime.ToolOutput{Content: "LSP is not available. No language server configured.", IsError: true}, nil
	}
	var args struct {
		FilePath string `json:"file_path"`
		Line     int    `json:"line"`
		Character int   `json:"character"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return &runtime.ToolOutput{Content: "Invalid input: " + err.Error(), IsError: true}, nil
	}
	items, err := r.lspManager.IncomingCalls(ctx, args.FilePath, args.Line, args.Character)
	if err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("LSP error: %v", err), IsError: true}, nil
	}
	if len(items) == 0 {
		return &runtime.ToolOutput{Content: "No incoming calls found.", IsError: false}, nil
	}
	return &runtime.ToolOutput{Content: formatCallHierarchyItems(items), IsError: false}, nil
}

func (r *Registry) lspOutgoingCallsTool(ctx context.Context, input json.RawMessage) (*runtime.ToolOutput, error) {
	if r.lspManager == nil {
		return &runtime.ToolOutput{Content: "LSP is not available. No language server configured.", IsError: true}, nil
	}
	var args struct {
		FilePath string `json:"file_path"`
		Line     int    `json:"line"`
		Character int   `json:"character"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return &runtime.ToolOutput{Content: "Invalid input: " + err.Error(), IsError: true}, nil
	}
	items, err := r.lspManager.OutgoingCalls(ctx, args.FilePath, args.Line, args.Character)
	if err != nil {
		return &runtime.ToolOutput{Content: fmt.Sprintf("LSP error: %v", err), IsError: true}, nil
	}
	if len(items) == 0 {
		return &runtime.ToolOutput{Content: "No outgoing calls found.", IsError: false}, nil
	}
	return &runtime.ToolOutput{Content: formatCallHierarchyItems(items), IsError: false}, nil
}

// formatSymbolLocations formats symbol locations as a readable string.
func formatSymbolLocations(locs []lsp.SymbolLocation) string {
	var sb strings.Builder
	for i, loc := range locs {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("%s:%d:%d", loc.Path, loc.Line, loc.Col))
	}
	return sb.String()
}

// formatCallHierarchyItems formats call hierarchy items as a readable string.
func formatCallHierarchyItems(items []lsp.CallHierarchyItem) string {
	var sb strings.Builder
	for i, item := range items {
		if i > 0 {
			sb.WriteString("\n")
		}
		path := strings.TrimPrefix(item.URI, "file://")
		sb.WriteString(fmt.Sprintf("%s:%d:%d - %s", path, item.Range.Start.Line+1, item.Range.Start.Character+1, item.Name))
	}
	return sb.String()
}
