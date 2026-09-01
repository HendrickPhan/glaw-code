package transport

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// fakeMCPServer is a minimal spec-compliant MCP stdio server: JSON-RPC
// messages are newline-delimited, with no Content-Length framing.
const fakeMCPServer = `
import sys, json

for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    msg = json.loads(line)
    if 'id' not in msg:
        continue
    method = msg.get('method')
    if method == 'initialize':
        result = {"protocolVersion": "2024-11-05", "capabilities": {"tools": {}},
                  "serverInfo": {"name": "fake", "version": "1.0"}}
    elif method == 'tools/list':
        result = {"tools": [{"name": "echo", "description": "echo the input",
                             "inputSchema": {"type": "object"}}]}
    elif method == 'tools/call':
        result = {"content": [{"type": "text", "text": "echo:" + msg['params']['name']}]}
    else:
        result = {}
    sys.stdout.write(json.dumps({"jsonrpc": "2.0", "id": msg["id"], "result": result}) + "\n")
    sys.stdout.flush()
`

// hungMCPServer reads stdin but never responds, simulating an unresponsive server.
const hungMCPServer = `
import sys, time
time.sleep(300)
`

// writeServerScript materializes a python server script in a temp dir.
// The test is skipped when python3 is unavailable.
func writeServerScript(t *testing.T, body string) string {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	path := filepath.Join(t.TempDir(), "server.py")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("writing server script: %v", err)
	}
	return path
}

func TestStdioHandshakeAndToolCall(t *testing.T) {
	script := writeServerScript(t, fakeMCPServer)

	mgr := NewManager()
	configs := map[string]ServerConfig{
		"fake": {Transport: "stdio", Command: "python3", Args: []string{script}},
	}
	if err := mgr.InitializeAll(context.Background(), configs); err != nil {
		t.Fatalf("InitializeAll: %v", err)
	}

	if got := mgr.ToolCount(); got != 1 {
		t.Fatalf("ToolCount = %d, want 1", got)
	}
	if !mgr.HasTool("echo") {
		t.Fatal("HasTool(echo) = false, want true")
	}

	result, err := mgr.CallTool(context.Background(), "echo", map[string]interface{}{})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "echo:echo" {
		t.Fatalf("CallTool result = %+v, want text %q", result.Content, "echo:echo")
	}

	if err := mgr.Shutdown(); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestInitializeAllTimesOutOnUnresponsiveServer(t *testing.T) {
	script := writeServerScript(t, hungMCPServer)

	oldTimeout := initializeTimeout
	initializeTimeout = 500 * time.Millisecond
	t.Cleanup(func() { initializeTimeout = oldTimeout })

	mgr := NewManager()
	configs := map[string]ServerConfig{
		"hung": {Transport: "stdio", Command: "python3", Args: []string{script}},
	}

	start := time.Now()
	if err := mgr.InitializeAll(context.Background(), configs); err != nil {
		t.Fatalf("InitializeAll: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Fatalf("InitializeAll took %v; unresponsive server must not block startup", elapsed)
	}
	if mgr.ToolCount() != 0 {
		t.Fatalf("ToolCount = %d, want 0 for unresponsive server", mgr.ToolCount())
	}
	_ = mgr.Shutdown()
}

// TestConnectionOutlivesInitializeTimeout is a regression test: the child
// process must not be killed when the per-server initialize timeout expires
// after a successful handshake.
func TestConnectionOutlivesInitializeTimeout(t *testing.T) {
	script := writeServerScript(t, fakeMCPServer)

	oldTimeout := initializeTimeout
	initializeTimeout = 300 * time.Millisecond
	t.Cleanup(func() { initializeTimeout = oldTimeout })

	mgr := NewManager()
	configs := map[string]ServerConfig{
		"fake": {Transport: "stdio", Command: "python3", Args: []string{script}},
	}
	if err := mgr.InitializeAll(context.Background(), configs); err != nil {
		t.Fatalf("InitializeAll: %v", err)
	}

	// Wait well past the (already-satisfied) initialize deadline.
	time.Sleep(700 * time.Millisecond)

	result, err := mgr.CallTool(context.Background(), "echo", map[string]interface{}{})
	if err != nil {
		t.Fatalf("CallTool after init timeout expired: %v (server process was killed by the handshake deadline)", err)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "echo:echo" {
		t.Fatalf("CallTool result = %+v, want text %q", result.Content, "echo:echo")
	}
	_ = mgr.Shutdown()
}

// TestCallToolFailsWhenServerDies is a regression test: requests in flight
// (or issued after) when the server process exits must fail with an error,
// not hang forever.
func TestCallToolFailsWhenServerDies(t *testing.T) {
	script := writeServerScript(t, hungMCPServer)

	oldTimeout := initializeTimeout
	initializeTimeout = 300 * time.Millisecond
	t.Cleanup(func() { initializeTimeout = oldTimeout })

	mgr := NewManager()
	configs := map[string]ServerConfig{
		"hung": {Transport: "stdio", Command: "python3", Args: []string{script}},
	}
	_ = mgr.InitializeAll(context.Background(), configs)

	// Kill the (unresponsive) server process, then attempt a call on a
	// background context — it must return an error, not block.
	_ = mgr.Shutdown()

	done := make(chan error, 1)
	go func() {
		_, err := mgr.CallTool(context.Background(), "echo", map[string]interface{}{})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("CallTool unexpectedly succeeded against a dead server")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("CallTool hung on dead server; pending requests must fail when the connection dies")
	}
}
