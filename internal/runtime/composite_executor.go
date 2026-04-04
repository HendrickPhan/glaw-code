package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hieu-glaw/glaw-code/internal/api"
	"github.com/hieu-glaw/glaw-code/internal/mcp"
)

// CompositeToolExecutor delegates tool execution to both builtin tools and
// MCP server tools. Builtin tools take priority; if the builtin executor
// reports an unknown tool, the request is forwarded to the MCP manager.
type CompositeToolExecutor struct {
	builtin   ToolExecutor
	mcpManager *mcp.Manager
}

// NewCompositeToolExecutor creates a composite executor wrapping a builtin
// executor and an MCP manager. Either may be nil; a nil builtin is replaced
// with a no-op executor and a nil MCP manager is treated as having no tools.
func NewCompositeToolExecutor(builtin ToolExecutor, mcpManager *mcp.Manager) *CompositeToolExecutor {
	if builtin == nil {
		builtin = &noopToolExecutor{}
	}
	return &CompositeToolExecutor{
		builtin:   builtin,
		mcpManager: mcpManager,
	}
}

// GetToolSpecs merges builtin tool specs with MCP tool specs, converting
// each MCP ManagedTool into the api.ToolDefinition format.
func (e *CompositeToolExecutor) GetToolSpecs() []api.ToolDefinition {
	specs := e.builtin.GetToolSpecs()

	if e.mcpManager == nil {
		return specs
	}

	for _, t := range e.mcpManager.GetTools() {
		schema, err := json.Marshal(t.InputSchema)
		if err != nil {
			// Skip tools with non-serializable schemas.
			continue
		}
		specs = append(specs, api.ToolDefinition{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: json.RawMessage(schema),
		})
	}

	return specs
}

// ExecuteTool first tries the builtin executor. If the builtin reports an
// unknown tool error, it falls back to the MCP manager's CallTool.
func (e *CompositeToolExecutor) ExecuteTool(ctx context.Context, name string, input json.RawMessage) (*ToolOutput, error) {
	// Try builtin first.
	output, err := e.builtin.ExecuteTool(ctx, name, input)
	if err != nil {
		return nil, err
	}

	// Builtin handled it successfully (or with a known tool error that isn't "unknown").
	if !output.IsError || !isUnknownTool(output.Content, name) {
		return output, nil
	}

	// Fall back to MCP.
	if e.mcpManager == nil {
		return output, nil
	}

	// Convert raw JSON input to map[string]interface{} for MCP CallTool.
	var args map[string]interface{}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return nil, fmt.Errorf("parsing tool input for MCP tool %q: %w", name, err)
		}
	}
	if args == nil {
		args = make(map[string]interface{})
	}

	result, err := e.mcpManager.CallTool(ctx, name, args)
	if err != nil {
		// MCP doesn't know the tool either; return the original builtin error.
		return output, nil
	}

	// Concatenate text content blocks from the MCP result.
	var content strings.Builder
	for _, c := range result.Content {
		if c.Type == string(api.ContentText) {
			if content.Len() > 0 {
				content.WriteString("\n")
			}
			content.WriteString(c.Text)
		}
	}

	return &ToolOutput{
		Content: content.String(),
		IsError: result.IsError,
	}, nil
}

// isUnknownTool checks whether the output content matches the pattern used
// by BuiltinToolExecutor for unrecognised tools: "Unknown tool: <name>".
func isUnknownTool(content, name string) bool {
	return content == fmt.Sprintf("Unknown tool: %s", name)
}

// noopToolExecutor is a fallback executor that reports every tool as unknown.
type noopToolExecutor struct{}

func (n *noopToolExecutor) ExecuteTool(_ context.Context, name string, _ json.RawMessage) (*ToolOutput, error) {
	return &ToolOutput{Content: fmt.Sprintf("Unknown tool: %s", name), IsError: true}, nil
}

func (n *noopToolExecutor) GetToolSpecs() []api.ToolDefinition {
	return nil
}
