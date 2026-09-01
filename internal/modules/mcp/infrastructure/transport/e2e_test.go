package transport

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

// End-to-end tests against real MCP servers. Skipped unless
// GLAW_MCP_E2E=1 is set, so CI and offline runs don't depend on
// external binaries or network access.

func TestE2ERealStdioServer(t *testing.T) {
	if os.Getenv("GLAW_MCP_E2E") != "1" {
		t.Skip("set GLAW_MCP_E2E=1 to run end-to-end tests")
	}
	if _, err := exec.LookPath("codegraph"); err != nil {
		t.Skip("codegraph not installed")
	}

	mgr := NewManager()
	configs := map[string]ServerConfig{
		"codegraph": {Transport: "stdio", Command: "codegraph", Args: []string{"serve", "--mcp"}},
	}
	if err := mgr.InitializeAll(context.Background(), configs); err != nil {
		t.Fatalf("InitializeAll: %v", err)
	}
	if mgr.ToolCount() == 0 {
		t.Fatal("no tools discovered from real codegraph server")
	}
	t.Logf("codegraph: %d tools discovered", mgr.ToolCount())
	_ = mgr.Shutdown()
}

func TestE2ERealHTTPServer(t *testing.T) {
	if os.Getenv("GLAW_MCP_E2E") != "1" {
		t.Skip("set GLAW_MCP_E2E=1 to run end-to-end tests")
	}

	mgr := NewManager()
	configs := map[string]ServerConfig{
		"context7": {Transport: "http", URL: "https://mcp.context7.com/mcp"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := mgr.InitializeAll(ctx, configs); err != nil {
		t.Fatalf("InitializeAll: %v", err)
	}
	if mgr.ToolCount() == 0 {
		t.Fatal("no tools discovered from real context7 server")
	}
	t.Logf("context7: %d tools discovered", mgr.ToolCount())
	_ = mgr.Shutdown()
}
