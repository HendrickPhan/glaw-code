package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hieu-glaw/glaw-code/internal/commands"
	"github.com/hieu-glaw/glaw-code/internal/runtime"
)

// REPL manages the read-eval-print loop.
type REPL struct {
	Runtime  *runtime.ConversationRuntime
	CmdDisp  *commands.Dispatcher
	reader   *bufio.Reader

	// mu protects the interrupt counter
	mu             sync.Mutex
	interruptCount int
	interruptReset *time.Timer
	// shutdownCh is closed when the REPL should fully exit
	shutdownCh chan struct{}
	// shutdownOnce ensures we only close shutdownCh once
	shutdownOnce sync.Once

	// Command history
	history *InputHistory

	// Background action state
	bgAction     *backgroundAction
	bgMu         sync.Mutex
	thinkingDetail bool

	// activeSpinner holds the currently active CLI spinner (e.g. "Thinking...")
	// so it can be paused when a permission prompt needs to interact with the user.
	activeSpinner *Spinner
}

// backgroundAction holds state for an action moved to background via /btw.
type backgroundAction struct {
	actionCancel context.CancelFunc
	resultCh     chan runResult
}

// runResult holds the result of a background action.
type runResult struct {
	err error
}

// NewREPL creates a new REPL with permission checking wired in.
func NewREPL(rt *runtime.ConversationRuntime) *REPL {
	reader := bufio.NewReader(os.Stdin)

	repl := &REPL{
		Runtime:    rt,
		CmdDisp:    commands.NewDispatcher(rt),
		reader:     reader,
		shutdownCh: make(chan struct{}),
		history:    NewInputHistory(),
	}

	// Wire the permission checker: prompt user for dangerous operations.
	// The spinner is paused while prompting so it doesn't overwrite the prompt.
	rt.PermissionChecker = func(toolName string, input json.RawMessage) bool {
		// Pause the active spinner if one is running
		if repl.activeSpinner != nil {
			repl.activeSpinner.Stop()
		}
		result := checkToolPermission(rt, reader, toolName, input)
		return result
	}

	return repl
}

// Shutdown signals the REPL to exit gracefully.
func (r *REPL) Shutdown() {
	r.shutdownOnce.Do(func() {
		close(r.shutdownCh)
	})
}

// recordInterrupt records a Ctrl+C press and returns the new count.
// The counter auto-resets after 2 seconds of no interrupts.
func (r *REPL) recordInterrupt() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.interruptCount++

	// Reset the counter after 2 seconds of no interrupts
	if r.interruptReset != nil {
		r.interruptReset.Stop()
	}
	r.interruptReset = time.AfterFunc(2*time.Second, func() {
		r.mu.Lock()
		r.interruptCount = 0
		r.mu.Unlock()
	})

	return r.interruptCount
}

// resetInterrupt resets the interrupt counter.
func (r *REPL) resetInterrupt() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.interruptCount = 0
	if r.interruptReset != nil {
		r.interruptReset.Stop()
	}
}

// askYesNo prompts the user with a yes/no question and returns the answer.
func askYesNo(reader *bufio.Reader, question string) bool {
	fmt.Printf("%s", question)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "y" || answer == "yes"
}

// checkToolPermission determines if a tool invocation needs user approval.
func checkToolPermission(rt *runtime.ConversationRuntime, reader *bufio.Reader, toolName string, input json.RawMessage) bool {
	mode := rt.Permissions.Mode

	// Yolo mode: auto-approve everything
	if mode == runtime.PermYolo {
		return true
	}

	switch mode {
	case runtime.PermDangerFullAccess, runtime.PermAllow:
		return true

	case runtime.PermReadOnly:
		switch toolName {
		case "read_file", "search_files", "list_directory", "get_file_info":
			return true
		default:
			fmt.Printf("\n%sPermission denied:%s tool %q not allowed in read_only mode.\n", Red, Reset, toolName)
			return false
		}

	case runtime.PermWorkspaceWrite:
		switch toolName {
		case "bash":
			// Always prompt for bash in workspace_write mode
			display := FormatToolInput(toolName, input)
			fmt.Println()
			fmt.Printf("%s%sPermission Required%s\n", Bold+Yellow, ">> ", Reset)
			fmt.Printf("%sTool:%s %s%s%s\n", Bold, Reset, Cyan, toolName, Reset)
			fmt.Printf("%sCommand:%s %s\n", Dim, Reset, display)
			return askYesNo(reader, fmt.Sprintf("%sAllow? [y/n]:%s ", Green, Reset))

		case "write_file", "edit_file":
			// Validate path is within workspace
			var args struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(input, &args); err == nil && args.Path != "" {
				err := runtime.ValidatePathWithinWorkspace(args.Path, rt.Permissions.WorkspaceRoot)
				if err != nil {
					fmt.Println()
					fmt.Printf("%s%sPath outside workspace:%s %s\n", Bold+Yellow, ">> ", Reset, err.Error())
					return askYesNo(reader, fmt.Sprintf("%sAllow anyway? [y/n]:%s ", Green, Reset))
				}
			}
			return true

		default:
			return true
		}

	default:
		// Unknown mode: prompt for everything
		display := FormatToolInput(toolName, input)
		fmt.Println()
		fmt.Printf("%s%sPermission Required%s\n", Bold+Yellow, ">> ", Reset)
		fmt.Printf("%sTool:%s %s%s%s\n", Bold, Reset, Cyan, toolName, Reset)
		fmt.Printf("%sInput:%s %s\n", Dim, Reset, display)
		return askYesNo(reader, fmt.Sprintf("%sAllow? [y/n]:%s ", Green, Reset))
	}
}

// gracefulShutdown performs cleanup before exiting.
func (r *REPL) gracefulShutdown() {
	fmt.Println()

	// Cancel any background action
	r.bgMu.Lock()
	if r.bgAction != nil {
		r.bgAction.actionCancel()
		r.bgAction = nil
	}
	r.bgMu.Unlock()

	// Save session
	if r.Runtime.Session != nil && r.Runtime.Session.ID != "" {
		workspaceRoot := r.Runtime.GetWorkspaceRoot()
		if workspaceRoot != "" {
			if path, err := runtime.SaveSession(r.Runtime.Session, filepath.Join(workspaceRoot, ".glaw", "sessions")); err == nil {
				fmt.Printf("%s  Session saved to %s%s\n", Dim, path, Reset)
			}
		}
	}

	// Shutdown LSP
	if r.Runtime.LSPManager != nil {
		_ = r.Runtime.LSPManager.Shutdown()
	}

	fmt.Printf("%sGoodbye!%s\n", Bold+Cyan, Reset)
}

// Run starts the REPL loop with graceful Ctrl+C handling.
func (r *REPL) Run(ctx context.Context) error {
	// Set up signal channel
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Watch for the parent context being done
	go func() {
		<-ctx.Done()
		r.Shutdown()
	}()

	cwd, _ := os.Getwd()
	displayDir := cwd
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(cwd, home) {
		displayDir = "~" + strings.TrimPrefix(cwd, home)
	}

	fmt.Printf("%sglaw-code v1.0.0%s\n", Bold+Cyan, Reset)
	fmt.Printf("%sWorkspace:%s %s\n", Dim, Reset, displayDir)
	fmt.Printf("%sPermissions:%s %s\n", Dim, Reset, r.Runtime.Permissions.Mode)
	fmt.Printf("Type %s/%s for commands. %sCtrl+C%s to cancel/exit.\n", Bold, Reset, Dim, Reset)
	fmt.Println()

	for {
		select {
		case <-r.shutdownCh:
			r.gracefulShutdown()
			return nil
		default:
		}

		shortDir := filepath.Base(displayDir)

		// Check if there's a background action running
		r.bgMu.Lock()
		hasBg := r.bgAction != nil
		r.bgMu.Unlock()

		var promptStr string
		if hasBg {
			promptStr = fmt.Sprintf("%s%s⚡>%s ", Yellow+Bold, shortDir, Reset)
		} else {
			promptStr = fmt.Sprintf("%s%s>%s ", Green+Bold, shortDir, Reset)
		}

		input, err := ReadLineWithCompletionAndHistory(promptStr, r.history)
		if err != nil {
			if err == ErrInterrupted {
				count := r.recordInterrupt()
				if count >= 2 {
					r.gracefulShutdown()
					return nil
				}
				fmt.Printf("%s  Press Ctrl+C again within 2s to exit.%s\n", Yellow, Reset)
				continue
			}
			r.gracefulShutdown()
			return nil
		}

		// Handle Ctrl+O (thinking detail toggle)
		if input == "\x0f" {
			r.thinkingDetail = !r.thinkingDetail
			if r.thinkingDetail {
				fmt.Printf("%s  Thinking detail: ON (future thinking output will be expanded)%s\n", Cyan, Reset)
			} else {
				fmt.Printf("%s  Thinking detail: OFF%s\n", Dim, Reset)
			}
			continue
		}

		// Reset interrupt counter on successful input
		r.resetInterrupt()

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		if input == "/quit" || input == "/exit" || input == ":q" {
			r.gracefulShutdown()
			return nil
		}

		// Handle /fg command to bring background action back
		if input == "/fg" {
			r.bgMu.Lock()
			bg := r.bgAction
			r.bgMu.Unlock()
			if bg == nil {
				fmt.Printf("%s  No background action running.%s\n", Yellow, Reset)
				continue
			}
			// Wait for the background action to complete
			r.handleBackgroundResult(bg, sigChan)
			continue
		}

		// Handle /btw command to move action to background and ask new question
		if strings.HasPrefix(input, "/btw") {
			remainder := strings.TrimSpace(strings.TrimPrefix(input, "/btw"))
			if remainder == "" {
				fmt.Printf("%s  Usage: /btw <question>%s\n", Dim, Reset)
				continue
			}

			// If there's a running action, move it to background
			r.bgMu.Lock()
			if r.bgAction == nil {
				r.bgMu.Unlock()
				// No running action, just treat as a regular question
				if err := r.handlePromptWithCancel(ctx, sigChan, remainder); err != nil {
					select {
					case <-r.shutdownCh:
						r.gracefulShutdown()
						return nil
					default:
					}
					fmt.Printf("Error: %v\n", err)
				}
				continue
			}
			r.bgMu.Unlock()

			fmt.Printf("%s  ⚡ Current task moved to background. Type %s/fg%s to check on it.%s\n", Yellow, Bold, Yellow, Reset)
			fmt.Printf("%s  Asking your side question...%s\n", Cyan, Reset)

			// Handle the side question
			if err := r.handlePromptWithCancel(ctx, sigChan, remainder); err != nil {
				select {
				case <-r.shutdownCh:
					r.gracefulShutdown()
					return nil
				default:
				}
				fmt.Printf("Error: %v\n", err)
			}
			continue
		}

		if strings.HasPrefix(input, "/") {
			result, cmdErr := r.CmdDisp.Handle(ctx, input)
			if cmdErr != nil {
				fmt.Printf("Error: %v\n", cmdErr)
				continue
			}
			if result != nil {
				fmt.Println(result.Message)
				if result.Action == "quit" {
					r.gracefulShutdown()
					return nil
				}
			}
			continue
		}

		// Handle a prompt
		if err := r.handlePromptWithCancel(ctx, sigChan, input); err != nil {
			select {
			case <-r.shutdownCh:
				r.gracefulShutdown()
				return nil
			default:
			}
			fmt.Printf("Error: %v\n", err)
		}
	}
}

// handleBackgroundResult waits for a background action to complete and displays its result.
func (r *REPL) handleBackgroundResult(bg *backgroundAction, sigChan <-chan os.Signal) {
	fmt.Printf("%s  ⚡ Waiting for background task...%s\n", Yellow, Reset)

	// Show spinner while waiting
	spin := NewSpinner("Background task running...")

	select {
	case result := <-bg.resultCh:
		spin.Stop()
		r.bgMu.Lock()
		r.bgAction = nil
		r.bgMu.Unlock()

		if result.err != nil {
			if runtime.IsActionCancelled(result.err) {
				fmt.Printf("%s  Background action cancelled.%s\n", Green, Reset)
			} else {
				fmt.Printf("%s  Background error: %v%s\n", Red, result.err, Reset)
			}
		} else {
			fmt.Printf("%s  ✓ Background task completed.%s\n", Green, Reset)
			displayUsage(r.Runtime)
		}

		if r.Runtime.Snapshotter != nil {
			r.Runtime.Snapshotter.FinishBatch()
		}

		if path, err := runtime.SaveSession(r.Runtime.Session, ".glaw/sessions"); err == nil {
			_ = path
		}

	case sig := <-sigChan:
		spin.Stop()
		_ = sig
		fmt.Printf("\n%s  Cancelling background task...%s\n", Yellow, Reset)
		bg.actionCancel()

		// Wait for it to finish
		select {
		case <-bg.resultCh:
		case <-time.After(3 * time.Second):
		}

		r.bgMu.Lock()
		r.bgAction = nil
		r.bgMu.Unlock()

		fmt.Printf("%s  Background task cancelled.%s\n", Green, Reset)

	case <-r.shutdownCh:
		spin.Stop()
		bg.actionCancel()
	}
}

// handlePromptWithCancel runs the prompt with Ctrl+C cancellation support.
func (r *REPL) handlePromptWithCancel(ctx context.Context, sigChan <-chan os.Signal, prompt string) error {
	// Create a cancellable context for this action
	actionCtx, actionCancel := context.WithCancel(ctx)
	defer actionCancel()

	// Mark runtime as running
	r.Runtime.SetRunning(actionCancel)
	defer r.Runtime.SetIdle()

	r.Runtime.Session.AddUserMessageFromText(prompt)

	if r.Runtime.Snapshotter != nil {
		r.Runtime.Snapshotter.BeginBatch()
	}

	// Run the action in a goroutine so we can handle signals and user input
	resultCh := make(chan runResult, 1)

	go func() {
		err := r.Runtime.RunLoop(actionCtx)
		resultCh <- runResult{err: err}
	}()

	// Show thinking spinner
	spin := NewSpinner("Thinking...")
	r.activeSpinner = spin
	defer func() {
		spin.Stop()
		r.activeSpinner = nil
	}()

	// Wait for either the action to complete or a signal
	var runErr error
	secondSig := false

	for {
		select {
		case result := <-resultCh:
			runErr = result.err
			spin.Stop()
			goto done

		case sig := <-sigChan:
			_ = sig
			if !secondSig {
				fmt.Printf("\n%s  Cancelling action...%s\n", Yellow, Reset)
				actionCancel()
				secondSig = true
			} else {
				fmt.Printf("\n%s  Force exit requested.%s\n", Red, Reset)
				r.Shutdown()
				return nil
			}

		case <-r.shutdownCh:
			actionCancel()
			return nil
		}
	}

done:
	if r.Runtime.Snapshotter != nil {
		r.Runtime.Snapshotter.FinishBatch()
	}

	if runErr != nil {
		if runtime.IsActionCancelled(runErr) {
			fmt.Printf("%s  Action cancelled. Returning to prompt.%s\n", Green, Reset)
			return nil
		}
		return runErr
	}

	displayUsage(r.Runtime)

	if path, err := runtime.SaveSession(r.Runtime.Session, ".glaw/sessions"); err == nil {
		_ = path
	}

	return nil
}

// RunOneShot executes a single prompt without entering the REPL.
func RunOneShot(ctx context.Context, rt *runtime.ConversationRuntime, prompt string) error {
	reader := bufio.NewReader(os.Stdin)
	rt.PermissionChecker = func(toolName string, input json.RawMessage) bool {
		return checkToolPermission(rt, reader, toolName, input)
	}

	if rt.Snapshotter != nil {
		rt.Snapshotter.BeginBatch()
	}

	rt.Session.AddUserMessageFromText(prompt)

	result, err := rt.Turn(ctx)
	if err != nil {
		return err
	}

	for _, block := range result.Response.Content {
		if block.Type == "text" {
			fmt.Println(RenderMarkdown(block.Text))
		}
	}

	if result.StopReason == "tool_use" {
		err := rt.RunToolLoop(ctx, result)
		if rt.Snapshotter != nil {
			rt.Snapshotter.FinishBatch()
		}
		return err
	}

	if rt.Snapshotter != nil {
		rt.Snapshotter.FinishBatch()
	}

	displayUsage(rt)
	return nil
}

func displayUsage(rt *runtime.ConversationRuntime) {
	_, _, total := rt.Usage.EstimateCost(rt.Config.Model)
	if total > 0 {
		fmt.Println()
		fmt.Println(RenderUsage(rt.Usage.Cumulative.InputTokens, rt.Usage.Cumulative.OutputTokens, total))
	}
}
