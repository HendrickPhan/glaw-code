// Package bootstrap provides the application composition root.
// It wires together all modules using constructor injection,
// following Clean Architecture dependency rules.
package bootstrap

import (
	"context"
	"fmt"
	"os"

	"github.com/hieu-glaw/glaw-code/internal/api"
	agentusecase "github.com/hieu-glaw/glaw-code/internal/modules/agent/application/usecase"
	agententity "github.com/hieu-glaw/glaw-code/internal/modules/agent/domain/entity"
	config "github.com/hieu-glaw/glaw-code/internal/modules/config/domain/entity"
	conventity "github.com/hieu-glaw/glaw-code/internal/modules/conversation/domain/entity"
	mcp "github.com/hieu-glaw/glaw-code/internal/modules/mcp/infrastructure/transport"
	permservice "github.com/hieu-glaw/glaw-code/internal/modules/permission/domain/service"
	sessionentity "github.com/hieu-glaw/glaw-code/internal/modules/session/domain/entity"
	tools "github.com/hieu-glaw/glaw-code/internal/modules/tools/infrastructure/registry"
)

// MCPConfig represents an MCP server configuration for bootstrap purposes.
type MCPConfig struct {
	Transport string
	Command   string
	Args      []string
	URL       string
	Headers   map[string]string
	Env       map[string]string
}

// AppConfig holds all configuration needed to bootstrap the application.
type AppConfig struct {
	Settings      config.Settings
	RuntimeConfig *conventity.Config
	APIClient     api.ProviderClient
	MCPManager    *mcp.Manager
	ToolRegistry  *tools.Registry
	PermManager   *permservice.PermissionManager
	WorkspaceRoot string
}

// LoadConfig loads and layers configuration from all sources.
func LoadConfig(workspaceRoot string, modelOverride string, permOverride string, configPath string) (config.Settings, *conventity.Config, error) {
	settings, err := config.LoadAll(workspaceRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: loading settings: %v\n", err)
		settings = config.DefaultSettings()
	}

	if configPath != "" {
		explicit, err := config.LoadFromFile(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: loading config %s: %v\n", configPath, err)
		} else if explicit != nil {
			settings = config.Merge(settings, *explicit)
		}
	}

	if modelOverride != "" {
		settings.Model = modelOverride
	}
	if permOverride != "" {
		settings.Permissions.Mode = permOverride
	}

	cfg := conventity.ConfigFromSettings(settings)
	return settings, cfg, nil
}

// CreateAPIClient creates an API client for the given model.
func CreateAPIClient(model string) (api.ProviderClient, error) {
	return api.NewProviderClient(model)
}

// SetupMCP initializes MCP server connections.
func SetupMCP(ctx context.Context, servers map[string]*config.MCPServerConfig) *mcp.Manager {
	mgr := mcp.NewManager()
	mcpConfigs := ConvertMCPConfigs(servers)
	if err := mgr.InitializeAll(ctx, mcpConfigs); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: initializing MCP servers: %v\n", err)
	}
	return mgr
}

// SetupTools creates the tool registry and wires sub-agents.
func SetupTools(workspaceRoot string, model string, apiClient api.ProviderClient) *tools.Registry {
	registry := tools.NewRegistry(workspaceRoot)

	// Load custom sub-agent configs
	customAgents, err := agententity.LoadAllSubAgents(workspaceRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: loading sub-agent configs: %v\n", err)
	}
	if len(customAgents) > 0 {
		agentusecase.SetCustomConfigs(customAgents)
	}

	// Create and wire the orchestrator
	specs := registry.GetToolSpecs()
	orch := agentusecase.NewSubAgentOrchestratorWithClient(registry, specs, model, apiClient)
	registry.SetOrchestrator(orch)

	return registry
}

// CreateRuntime creates the full conversation runtime with all dependencies.
func CreateRuntime(
	client api.ProviderClient,
	cfg *conventity.Config,
	permManager *permservice.PermissionManager,
	toolExec conventity.ToolExecutor,
) *conventity.ConversationRuntime {
	rt := conventity.NewConversationRuntime(client, cfg, sessionentity.NewSession(), permManager, toolExec)
	rt.ClientFactory = func(model string) (api.ProviderClient, error) {
		return api.NewProviderClient(model)
	}
	return rt
}

// ConvertMCPConfigs converts config.MCPServerConfig values to mcp.ServerConfig.
func ConvertMCPConfigs(servers map[string]*config.MCPServerConfig) map[string]mcp.ServerConfig {
	result := make(map[string]mcp.ServerConfig)
	for name, cfg := range servers {
		if cfg == nil {
			continue
		}
		result[name] = mcp.ServerConfig{
			Transport: cfg.Transport,
			Command:   cfg.Command,
			Args:      cfg.Args,
			URL:       cfg.URL,
			Headers:   cfg.Headers,
			Env:       cfg.Env,
		}
	}
	return result
}
