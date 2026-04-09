package web

import (
	"fmt"
	"log"
	"net/http"
	"os/exec"
	runtimestd "runtime"
)

// ServeOpts holds options for the serve command.
type ServeOpts struct {
	Addr           string
	Open           bool
	RuntimeFactory RuntimeFactory
	WorkspaceRoot  string
}

// Serve starts the web server and optionally opens a browser.
func Serve(opts ServeOpts) error {
	srv := NewWebServer(opts.RuntimeFactory, opts.WorkspaceRoot)

	log.Printf("glaw-code web server starting on %s", opts.Addr)
	fmt.Printf("glaw-code web interface: http://localhost%s\n", opts.Addr)

	if opts.Open {
		go openBrowser("http://localhost" + opts.Addr)
	}

	return http.ListenAndServe(opts.Addr, srv.Handler())
}

// openBrowser opens the default browser to the given URL.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtimestd.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		log.Printf("Cannot auto-open browser on %s", runtimestd.GOOS)
		return
	}
	if err := cmd.Run(); err != nil {
		log.Printf("Failed to open browser: %v", err)
	}
}
