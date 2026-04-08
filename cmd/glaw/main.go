package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/hieu-glaw/glaw-code/internal/agent"
	"github.com/hieu-glaw/glaw-code/internal/api"
	"github.com/hieu-glaw/glaw-code/internal/cli"
	"github.com/hieu-glaw/glaw-code/internal/config"
	"github.com/hieu-glaw/glaw-code/internal/mcp"
	"github.com/hieu-glaw/glaw-code/internal/runtime"
	"github.com/hieu-glaw/glaw-code/internal/tools"
	"github.com/hieu-glaw/glaw-code/internal/web"
)

// Version is set at build time via -ldflags "-X main.Version=...".
var Version = "dev"

func main() {
	// Handle serve subcommand
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
		serveAddr := serveCmd.String("addr", ":8080", "Address to listen on")
		serveOpen := serveCmd.Bool("open", true, "Open browser automatically")
		serveModel := serveCmd.String("model", "", "Model to use")
		serveConfigPath := serveCmd.String("config", "", "Path to config file")
		if err := serveCmd.Parse(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing serve flags: %v\n", err)
			os.Exit(1)
		}

		workspaceRoot, _ := os.Getwd()
		settings, err := config.LoadAll(workspaceRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: loading settings: %v\n", err)
			settings = config.DefaultSettings()
		}
		if *serveConfigPath != "" {
			explicit, err := config.LoadFromFile(*serveConfigPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: loading config %s: %v\n", *serveConfigPath, err)
			} else if explicit != nil {
				settings = config.Merge(settings, *explicit)
			}
		}
		if *serveModel != "" {
			settings.Model = *serveModel
		}
		cfg := runtime.ConfigFromSettings(settings)

		client, err := api.NewProviderClient(cfg.Model)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating API client: %v\n", err)
			fmt.Fprintf(os.Stderr, "Set the appropriate API key environment variable or configure it in ~/.glaw/settings.json.\n")
			fmt.Fprintf(os.Stderr, "Supported: OPENROUTER_API_KEY, ANTHROPIC_API_KEY, OPENAI_API_KEY, GEMINI_API_KEY, XAI_API_KEY, or use ollama: prefix for local models.\n")
			os.Exit(1)
		}

		mcpManager := mcp.NewManager()
		mcpConfigs := convertMCPConfigs(settings.MCPServers)
		ctx := context.Background()
		if err := mcpManager.InitializeAll(ctx, mcpConfigs); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: initializing MCP servers: %v\n", err)
		}

		runtimeFactory := func(sess *runtime.Session) (*runtime.ConversationRuntime, func(), error) {
			toolRegistry := tools.NewRegistry(workspaceRoot)
			setupSubAgents(toolRegistry, workspaceRoot, cfg.Model, client)
			snapshotExec := runtime.NewSnapshottingExecutor(toolRegistry)
			toolExec := runtime.NewCompositeToolExecutor(snapshotExec, mcpManager)
			permManager := runtime.NewPermissionManager(cfg.PermissionMode, workspaceRoot)
			rt := runtime.NewConversationRuntime(client, cfg, sess, permManager, toolExec)
			rt.Snapshotter = snapshotExec
			rt.ClientFactory = func(model string) (api.ProviderClient, error) {
				return api.NewProviderClient(model)
			}
			cleanup := func() {
				snapshotExec.FinishBatch()
			}
			return rt, cleanup, nil
		}

		opts := web.ServeOpts{
			Addr:           *serveAddr,
			Open:           *serveOpen,
			RuntimeFactory: runtimeFactory,
		}
		if err := web.Serve(opts); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Parse flags
	var (
		model       string
		permissions string
		sessionID   string
		configPath  string
		showVersion bool
		noInput     bool
	)

	flag.StringVar(&model, "model", "", "Model to use (e.g. claude-sonnet-4-6, gpt-4o, gemini-2.5-pro, grok-3, ollama:llama3)")
	flag.StringVar(&permissions, "permissions", "", "Permission mode (read_only, workspace_write, danger_full_access)")
	flag.StringVar(&sessionID, "session", "", "Session ID to resume")
	flag.StringVar(&configPath, "config", "", "Path to config file")
	flag.BoolVar(&showVersion, "version", false, "Show version")
	flag.BoolVar(&noInput, "no-input", false, "Non-interactive mode")
	flag.Parse()

	if showVersion {
		fmt.Printf("glaw-code %s\n", Version)
		os.Exit(0)
	}

	// Get remaining args as prompt
	prompt := ""
	args := flag.Args()
	if len(args) > 0 {
		prompt = args[0]
	}

	// Load config (layered: defaults -> global ~/.glaw/settings.json -> project .glaw/settings.json -> explicit --config -> CLI flags)
	workspaceRoot, _ := os.Getwd()
	settings, err := config.LoadAll(workspaceRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: loading settings: %v\n", err)
		settings = config.DefaultSettings()
	}

	// If an explicit --config path is given, layer it on top
	if configPath != "" {
		explicit, err := config.LoadFromFile(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: loading config %s: %v\n", configPath, err)
		} else if explicit != nil {
			settings = config.Merge(settings, *explicit)
		}
	}

	// Apply CLI flag overrides
	if model != "" {
		settings.Model = model
	}
	if permissions != "" {
		settings.Permissions.Mode = permissions
	}

	cfg := runtime.ConfigFromSettings(settings)

	// Create API client
	client, err := api.NewProviderClient(cfg.Model)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating API client: %v\n", err)
		fmt.Fprintf(os.Stderr, "Set the appropriate API key environment variable or configure it in ~/.glaw/settings.json.\n")
		fmt.Fprintf(os.Stderr, "Supported: OPENROUTER_API_KEY, ANTHROPIC_API_KEY, OPENAI_API_KEY, GEMINI_API_KEY, XAI_API_KEY, or use ollama: prefix for local models.\n")
		os.Exit(1)
	}

	// Create or load session
	session := runtime.NewSession()
	if sessionID != "" {
		loaded, err := runtime.LoadSession(filepath.Join(".glaw", "sessions", sessionID+".json"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: loading session: %v\n", err)
		} else {
			session = loaded
		}
	}

	// Create permission manager
	permManager := runtime.NewPermissionManager(cfg.PermissionMode, workspaceRoot)

	ctx := context.Background()

	// Initialize MCP manager
	mcpManager := mcp.NewManager()
	mcpConfigs := convertMCPConfigs(settings.MCPServers)
	if err := mcpManager.InitializeAll(ctx, mcpConfigs); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: initializing MCP servers: %v\n", err)
	}

	// Create tool executor (builtin tools + MCP)
	toolRegistry := tools.NewRegistry(workspaceRoot)
	setupSubAgents(toolRegistry, workspaceRoot, cfg.Model, client)
	snapshotExec := runtime.NewSnapshottingExecutor(toolRegistry)
	toolExec := runtime.NewCompositeToolExecutor(snapshotExec, mcpManager)

	// Create conversation runtime
	rt := runtime.NewConversationRuntime(client, cfg, session, permManager, toolExec)
	rt.Snapshotter = snapshotExec
	rt.ClientFactory = func(model string) (api.ProviderClient, error) {
		return api.NewProviderClient(model)
	}

	// Run
	if prompt != "" {
		// One-shot mode: set up signal handler for graceful shutdown
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-sigChan
			fmt.Println("\nInterrupted. Saving session...")
			_ = mcpManager.Shutdown()
			if path, err := runtime.SaveSession(session, filepath.Join(workspaceRoot, ".glaw", "sessions")); err == nil {
				fmt.Printf("Session saved to %s\n", path)
			}
			cancel()
		}()

		if err := cli.RunOneShot(ctx, rt, prompt); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	} else if !noInput {
		// Interactive REPL — signal handling is managed by the REPL itself
		repl := cli.NewREPL(rt)

		// Wire the agents provider for /agents command support
		agentMgr := agent.NewManager(rt)
		agentsProvider := agent.NewAgentsProviderAdapter(agentMgr)
		repl.SetAgentsProvider(agentsProvider)

		if err := repl.Run(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		// Graceful cleanup after REPL exits
		_ = mcpManager.Shutdown()
	} else {
		fmt.Fprintln(os.Stderr, "No prompt provided and --no-input set. Nothing to do.")
		os.Exit(1)
	}
}

// convertMCPConfigs converts config.MCPServerConfig values to mcp.ServerConfig.
func convertMCPConfigs(servers map[string]*config.MCPServerConfig) map[string]mcp.ServerConfig {
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

// setupSubAgents loads custom sub-agent configs from disk and wires the
// orchestrator into the tool registry so the sub_agent tool works.
func setupSubAgents(reg *tools.Registry, workspaceRoot string, model string, apiClient api.ProviderClient) {
	// Load custom agent configs from .glaw/agents/ (project) and ~/.glaw/agents/ (user)
	customAgents, err := agent.LoadAllSubAgents(workspaceRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: loading sub-agent configs: %v\n", err)
	}
	if len(customAgents) > 0 {
		agent.SetCustomConfigs(customAgents)
	}

	// Create and wire the orchestrator (with API client for real LLM-backed execution)
	specs := reg.GetToolSpecs()
	orch := agent.NewSubAgentOrchestratorWithClient(reg, specs, model, apiClient)
	reg.SetOrchestrator(orch)
}
