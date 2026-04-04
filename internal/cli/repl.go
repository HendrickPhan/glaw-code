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
	"syscall"

	"github.com/hieu-glaw/glaw-code/internal/commands"
	"github.com/hieu-glaw/glaw-code/internal/runtime"
)

// REPL manages the read-eval-print loop.
type REPL struct {
	Runtime  *runtime.ConversationRuntime
	CmdDisp  *commands.Dispatcher
	reader   *bufio.Reader
}

// NewREPL creates a new REPL with permission checking wired in.
func NewREPL(rt *runtime.ConversationRuntime) *REPL {
	reader := bufio.NewReader(os.Stdin)

	// Wire the permission checker: prompt user for dangerous operations
	rt.PermissionChecker = func(toolName string, input json.RawMessage) bool {
		return checkToolPermission(rt, reader, toolName, input)
	}

	return &REPL{
		Runtime: rt,
		CmdDisp: commands.NewDispatcher(rt),
		reader:  reader,
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

// Run starts the REPL loop.
func (r *REPL) Run(ctx context.Context) error {
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cwd, _ := os.Getwd()
	displayDir := cwd
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(cwd, home) {
		displayDir = "~" + strings.TrimPrefix(cwd, home)
	}

	fmt.Printf("%sglaw-code v1.0.0%s\n", Bold+Cyan, Reset)
	fmt.Printf("%sWorkspace:%s %s\n", Dim, Reset, displayDir)
	fmt.Printf("%sPermissions:%s %s\n", Dim, Reset, r.Runtime.Permissions.Mode)
	fmt.Println("Type / for commands with autocomplete. Press Ctrl+C to exit.")
	fmt.Println()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nGoodbye!")
			return nil
		default:
		}

		shortDir := filepath.Base(displayDir)
		promptStr := fmt.Sprintf("%s%s>%s ", Green+Bold, shortDir, Reset)
		input, err := ReadLineWithCompletion(promptStr)
		if err != nil {
			fmt.Println("\nGoodbye!")
			return nil
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		if input == "/quit" || input == "/exit" || input == ":q" {
			fmt.Println("Goodbye!")
			return nil
		}

		if strings.HasPrefix(input, "/") {
			result, err := r.CmdDisp.Handle(ctx, input)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				continue
			}
			if result != nil {
				fmt.Println(result.Message)
				if result.Action == "quit" {
					return nil
				}
			}
			continue
		}

		if err := r.handlePrompt(ctx, input); err != nil {
			fmt.Printf("Error: %v\n", err)
		}
	}
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

func (r *REPL) handlePrompt(ctx context.Context, prompt string) error {
	r.Runtime.Session.AddUserMessageFromText(prompt)

	if r.Runtime.Snapshotter != nil {
		r.Runtime.Snapshotter.BeginBatch()
	}

	err := r.Runtime.RunLoop(ctx)

	if r.Runtime.Snapshotter != nil {
		r.Runtime.Snapshotter.FinishBatch()
	}
	if err != nil {
		return err
	}

	displayUsage(r.Runtime)

	if path, err := runtime.SaveSession(r.Runtime.Session, ".glaw/sessions"); err == nil {
		_ = path
	}

	return nil
}

func displayUsage(rt *runtime.ConversationRuntime) {
	_, _, total := rt.Usage.EstimateCost(rt.Config.Model)
	if total > 0 {
		fmt.Println()
		fmt.Println(RenderUsage(rt.Usage.Cumulative.InputTokens, rt.Usage.Cumulative.OutputTokens, total))
	}
}
