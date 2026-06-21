package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hieu-glaw/glaw-code/internal/shared/api"
)

// BuiltinToolExecutor provides basic built-in tool execution.
type BuiltinToolExecutor struct {
	WorkspaceRoot string
}

// NewBuiltinToolExecutor creates a new tool executor.
func NewBuiltinToolExecutor(workspaceRoot string) *BuiltinToolExecutor {
	return &BuiltinToolExecutor{WorkspaceRoot: workspaceRoot}
}

// ExecuteTool dispatches to the appropriate tool handler.
func (e *BuiltinToolExecutor) ExecuteTool(ctx context.Context, name string, input json.RawMessage) (*ToolOutput, error) {
	switch name {
	case "bash":
		return e.executeBash(ctx, input)
	case "read_file":
		return e.executeReadFile(input)
	case "write_file":
		return e.executeWriteFile(input)
	case "edit_file":
		return e.executeEditFile(input)
	default:
		return &ToolOutput{Content: fmt.Sprintf("Unknown tool: %s", name), IsError: true}, nil
	}
}

// GetToolSpecs returns available tool definitions.
func (e *BuiltinToolExecutor) GetToolSpecs() []api.ToolDefinition {
	return []api.ToolDefinition{
		{Name: "bash", Description: "Execute shell commands", InputSchema: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`)},
		{Name: "read_file", Description: "Read file contents", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)},
		{Name: "write_file", Description: "Write file contents", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}}}`)},
		{Name: "edit_file", Description: "Edit file with string replacement", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"old_string":{"type":"string"},"new_string":{"type":"string"}}}`)},
	}
}

func (e *BuiltinToolExecutor) executeBash(ctx context.Context, input json.RawMessage) (*ToolOutput, error) {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("parsing bash input: %w", err)
	}

	cmd := exec.CommandContext(ctx, "bash", "-c", args.Command)
	cmd.Dir = e.WorkspaceRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n" + stderr.String()
	}
	if output == "" && err != nil {
		output = err.Error()
	}

	return &ToolOutput{Content: output, IsError: err != nil}, nil
}

func (e *BuiltinToolExecutor) executeReadFile(input json.RawMessage) (*ToolOutput, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("parsing read_file input: %w", err)
	}

	path := e.resolvePath(args.Path)
	data, err := os.ReadFile(path)
	if err != nil {
		return &ToolOutput{Content: fmt.Sprintf("Error reading file: %v", err), IsError: true}, nil
	}

	return &ToolOutput{Content: string(data)}, nil
}

func (e *BuiltinToolExecutor) executeWriteFile(input json.RawMessage) (*ToolOutput, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("parsing write_file input: %w", err)
	}

	path := e.resolvePath(args.Path)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &ToolOutput{Content: fmt.Sprintf("Error creating directory: %v", err), IsError: true}, nil
	}

	if err := os.WriteFile(path, []byte(args.Content), 0o644); err != nil {
		return &ToolOutput{Content: fmt.Sprintf("Error writing file: %v", err), IsError: true}, nil
	}

	return &ToolOutput{Content: fmt.Sprintf("Successfully wrote %s", path)}, nil
}

func (e *BuiltinToolExecutor) executeEditFile(input json.RawMessage) (*ToolOutput, error) {
	var args struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("parsing edit_file input: %w", err)
	}

	path := e.resolvePath(args.Path)
	data, err := os.ReadFile(path)
	if err != nil {
		return &ToolOutput{Content: fmt.Sprintf("Error reading file: %v", err), IsError: true}, nil
	}

	content := string(data)
	if !strings.Contains(content, args.OldString) {
		return &ToolOutput{Content: fmt.Sprintf("old_string not found in %s", path), IsError: true}, nil
	}

	newContent := strings.Replace(content, args.OldString, args.NewString, 1)
	if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
		return &ToolOutput{Content: fmt.Sprintf("Error writing file: %v", err), IsError: true}, nil
	}

	return &ToolOutput{Content: fmt.Sprintf("Successfully edited %s", path)}, nil
}

func (e *BuiltinToolExecutor) resolvePath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(e.WorkspaceRoot, p)
}
