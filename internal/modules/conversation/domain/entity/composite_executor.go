package entity

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	mcp "github.com/hieu-glaw/glaw-code/internal/modules/mcp/infrastructure/transport"
	"github.com/hieu-glaw/glaw-code/internal/shared/api"
)

// CompositeToolExecutor delegates tool execution to both builtin tools and
// MCP server tools.
type CompositeToolExecutor struct {
	builtin    ToolExecutor
	mcpManager *mcp.Manager
}

// NewCompositeToolExecutor creates a composite executor.
func NewCompositeToolExecutor(builtin ToolExecutor, mcpManager *mcp.Manager) *CompositeToolExecutor {
	if builtin == nil {
		builtin = &noopToolExecutor{}
	}
	return &CompositeToolExecutor{
		builtin:    builtin,
		mcpManager: mcpManager,
	}
}

// GetToolSpecs merges builtin tool specs with MCP tool specs.
func (e *CompositeToolExecutor) GetToolSpecs() []api.ToolDefinition {
	specs := e.builtin.GetToolSpecs()

	if e.mcpManager == nil {
		return specs
	}

	for _, t := range e.mcpManager.GetTools() {
		schema, err := json.Marshal(t.InputSchema)
		if err != nil {
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

// ExecuteTool first tries the builtin executor, then falls back to MCP.
func (e *CompositeToolExecutor) ExecuteTool(ctx context.Context, name string, input json.RawMessage) (*ToolOutput, error) {
	output, err := e.builtin.ExecuteTool(ctx, name, input)
	if err != nil {
		return nil, err
	}

	if !output.IsError || !isUnknownTool(output.Content, name) {
		return output, nil
	}

	if e.mcpManager == nil {
		return output, nil
	}

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
		return output, nil
	}

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

func isUnknownTool(content, name string) bool {
	return content == fmt.Sprintf("Unknown tool: %s", name)
}

type noopToolExecutor struct{}

func (n *noopToolExecutor) ExecuteTool(_ context.Context, name string, _ json.RawMessage) (*ToolOutput, error) {
	return &ToolOutput{Content: fmt.Sprintf("Unknown tool: %s", name), IsError: true}, nil
}

func (n *noopToolExecutor) GetToolSpecs() []api.ToolDefinition {
	return nil
}
