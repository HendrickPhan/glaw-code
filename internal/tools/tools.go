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
