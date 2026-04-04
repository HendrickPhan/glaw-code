package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/hieu-glaw/glaw-code/internal/runtime"
)

// PermissionPrompter handles interactive permission prompts in the REPL.
type PermissionPrompter struct {
	mu     sync.Mutex
	reader *bufio.Reader
}

// NewPermissionPrompter creates a new interactive permission prompter that
// reads from stdin.
func NewPermissionPrompter() *PermissionPrompter {
	return &PermissionPrompter{
		reader: bufio.NewReader(os.Stdin),
	}
}

// PromptForPermission displays a tool invocation request and asks the user
// whether to allow it. Returns true if the user approves.
//
// The user can respond with:
//   - "y" or "yes": allow this specific invocation
//   - "a" or "always": allow this tool for the rest of the session
//   - "n" or "no" (or empty/anything else): deny
func (p *PermissionPrompter) PromptForPermission(toolName string, input json.RawMessage) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	display := FormatToolInput(toolName, input)

	fmt.Println()
	fmt.Printf("%s%sPermission Required%s\n", Bold+Yellow, ">> ", Reset)
	fmt.Printf("%sTool:%s %s%s%s\n", Bold, Reset, Cyan, toolName, Reset)
	fmt.Printf("%sInput:%s %s\n", Dim, Reset, display)
	fmt.Println()
	fmt.Printf("%sAllow? [y]es / [a]lways / [n]o:%s ", Green, Reset)

	line, err := p.reader.ReadString('\n')
	if err != nil {
		return false
	}

	answer := strings.TrimSpace(strings.ToLower(line))
	switch answer {
	case "y", "yes":
		return true
	case "a", "always":
		return true
	default:
		return false
	}
}

// PromptChoice indicates the user's response to a permission prompt.
type PromptChoice int

const (
	ChoiceDeny   PromptChoice = iota // User denied
	ChoiceOnce                       // User approved this one time
	ChoiceAlways                     // User approved for the entire session
)

// PromptForPermissionChoice is like PromptForPermission but also returns
// whether the user chose "always" (session-level caching).
func (p *PermissionPrompter) PromptForPermissionChoice(toolName string, input json.RawMessage) (bool, PromptChoice) {
	p.mu.Lock()
	defer p.mu.Unlock()

	display := FormatToolInput(toolName, input)

	fmt.Println()
	fmt.Printf("%s%sPermission Required%s\n", Bold+Yellow, ">> ", Reset)
	fmt.Printf("%sTool:%s %s%s%s\n", Bold, Reset, Cyan, toolName, Reset)
	fmt.Printf("%sInput:%s %s\n", Dim, Reset, display)
	fmt.Println()
	fmt.Printf("%sAllow? [y]es / [a]lways / [n]o:%s ", Green, Reset)

	line, err := p.reader.ReadString('\n')
	if err != nil {
		return false, ChoiceDeny
	}

	answer := strings.TrimSpace(strings.ToLower(line))
	switch answer {
	case "y", "yes":
		return true, ChoiceOnce
	case "a", "always":
		return true, ChoiceAlways
	default:
		return false, ChoiceDeny
	}
}

// FormatToolInput formats a tool's input for display in the terminal.
// It pretty-prints JSON and truncates long content.
func FormatToolInput(toolName string, input json.RawMessage) string {
	if len(input) == 0 {
		return "(no input)"
	}

	// Try to pretty-print the JSON.
	var parsed map[string]interface{}
	if err := json.Unmarshal(input, &parsed); err != nil {
		// Not valid JSON object; just truncate the raw string.
		raw := string(input)
		if len(raw) > 300 {
			return raw[:300] + "..."
		}
		return raw
	}

	// Apply tool-specific formatting.
	switch toolName {
	case "bash":
		return formatBashInput(parsed)
	case "write_file":
		return formatWriteFileInput(parsed)
	case "edit_file":
		return formatEditFileInput(parsed)
	case "read_file":
		return formatReadFileInput(parsed)
	default:
		return formatGenericInput(parsed)
	}
}

// formatBashInput formats a bash command for display.
func formatBashInput(input map[string]interface{}) string {
	cmd, _ := input["command"].(string)
	if cmd == "" {
		return "(empty command)"
	}
	// Truncate very long commands.
	if len(cmd) > 300 {
		cmd = cmd[:300] + "..."
	}
	return cmd
}

// formatWriteFileInput formats a write_file input for display.
func formatWriteFileInput(input map[string]interface{}) string {
	path, _ := input["path"].(string)
	content, _ := input["content"].(string)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("path: %s", path))
	if content != "" {
		display := content
		if len(display) > 200 {
			display = display[:200] + "..."
		}
		sb.WriteString(fmt.Sprintf("\ncontent (%d bytes): %s", len(content), display))
	}
	return sb.String()
}

// formatEditFileInput formats an edit_file input for display.
func formatEditFileInput(input map[string]interface{}) string {
	path, _ := input["path"].(string)
	oldStr, _ := input["old_string"].(string)
	newStr, _ := input["new_string"].(string)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("path: %s", path))

	if oldStr != "" {
		display := oldStr
		if len(display) > 100 {
			display = display[:100] + "..."
		}
		sb.WriteString(fmt.Sprintf("\nreplace: %s", display))
	}
	if newStr != "" {
		display := newStr
		if len(display) > 100 {
			display = display[:100] + "..."
		}
		sb.WriteString(fmt.Sprintf("\nwith: %s", display))
	}
	return sb.String()
}

// formatReadFileInput formats a read_file input for display.
func formatReadFileInput(input map[string]interface{}) string {
	path, _ := input["path"].(string)
	return fmt.Sprintf("path: %s", path)
}

// formatGenericInput formats any tool input as truncated pretty JSON.
func formatGenericInput(input map[string]interface{}) string {
	data, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", input)
	}
	result := string(data)
	if len(result) > 500 {
		result = result[:500] + "..."
	}
	return result
}

// CheckAndPrompt is a convenience function that checks permission and prompts
// the user if needed. It returns a PermissionResult indicating the final decision.
// The enhancedPM and prompter can be nil; in those cases the function falls back
// to allowing everything.
func CheckAndPrompt(
	enhancedPM *runtime.EnhancedPermissionManager,
	prompter *PermissionPrompter,
	toolName string,
	requiredPerm runtime.Permission,
	input json.RawMessage,
) *runtime.PermissionResult {
	if enhancedPM == nil {
		return &runtime.PermissionResult{Allowed: true, Message: "no permission manager"}
	}

	result := enhancedPM.CheckToolPermission(toolName, requiredPerm, input)
	if result.Allowed {
		return result
	}

	// If the denial is definitive (not a prompt request), return as-is.
	if result.DenialReason != "needs_prompt" {
		return result
	}

	// Need to prompt the user.
	if prompter == nil {
		// No prompter available (non-interactive mode), deny.
		return &runtime.PermissionResult{
			Allowed:      false,
			Message:      "interactive prompt required but not available in non-interactive mode",
			DenialReason: "no_prompter",
		}
	}

	// Display the permission description and prompt.
	fmt.Println()
	fmt.Printf("%s%s%s\n", Yellow, result.Message, Reset)

	allowed, choice := prompter.PromptForPermissionChoice(toolName, input)
	if !allowed {
		return &runtime.PermissionResult{
			Allowed:      false,
			Message:      "user denied permission",
			DenialReason: "user_denied",
		}
	}

	// Record the decision for session-level caching.
	enhancedPM.RecordDecision(toolName, input, true)

	msg := "user approved"
	if choice == ChoiceAlways {
		// "Always" is handled by RecordDecision caching the key, so subsequent
		// calls with the same input will hit the cache.
		msg = "user approved (always)"
	}

	return &runtime.PermissionResult{
		Allowed: true,
		Message: msg,
	}
}
