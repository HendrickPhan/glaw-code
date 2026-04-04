# CLI, REPL, and Terminal UI

## Package: `cmd/claw/` + `internal/cli/`

---

## CLI Argument Parsing

### CliArgs

```go
type CliArgs struct {
    Prompt      string  // positional: the prompt (optional)
    Model       string  // --model
    Resume      string  // --resume (session ID to resume)
    SessionID   string  // --session-id
    Config      string  // --config (path to config file)
    Permissions string  // --permissions (mode override)
    Sandbox     string  // --sandbox
    NoInput     bool    // --no-input (non-interactive mode)
    Version     bool    // --version
}
```

---

## Main Flow

```
main():
  1. Parse CLI args
  2. IF --version: print version, exit
  3. Load config (layered merge)
  4. Apply CLI overrides (model, permissions, config path)
  5. Initialize runtime:
     a. Create API client based on model/provider
     b. Create tool executor with all tool sources
     c. Load or create session
     d. Initialize permission manager
     e. Initialize MCP server manager
     f. Initialize plugin manager
     g. Initialize LSP manager
  6. IF prompt provided as arg:
     Run single-turn execution:
       - Build system prompt
       - Send to API
       - Print response
       - Exit
  7. ELSE:
     Enter REPL loop
```

---

## REPL Loop

### StreamingState Machine

```go
type StreamingState int
// Idle → Streaming → (ToolUse → Streaming)* → Done
```

### ReplAction

```go
type ReplAction int
// Continue | Quit | SlashCommand
```

### Loop Logic

```
run_repl_loop(runtime):
  Print welcome banner
  LOOP:
    // 1. READ INPUT
    input = read_user_input()
    IF input is empty: continue
    IF input == "/quit" or "/exit": break
    IF input starts with "/":
      cmd, remainder = ParseSlashCommand(input)
      IF cmd != nil:
        result = HandleSlashCommand(cmd, remainder, runtime)
        IF result.Action == "quit": break
        continue
      ELSE:
        print "Unknown command. Did you mean: {suggestions}?"
        continue

    // 2. BUILD SYSTEM PROMPT
    systemPrompt = SystemPromptBuilder{
      ProjectContext:   build_project_context(),
      InstructionFiles: load_instruction_files(),
      GitStatus:        run_git_status(),
      GitDiff:          run_git_diff(),
      ToolDescriptions: build_tool_descriptions(runtime),
    }.Build()

    // 3. SEND TO API (streaming)
    messages = session.Messages
    append user message to messages
    stream = APIClient.StreamMessage(ApiRequest{
      Model:     config.Model,
      Messages:  messages,
      Tools:     build_tool_definitions(),
      MaxTokens: config.MaxTokens,
      System:    systemPrompt,
      Stream:    true,
    })

    // 4. PROCESS STREAM
    state = Idle
    FOR event := range stream:
      SWITCH event.Type:
      CASE "message_start":
        state = Streaming
        display start indicator

      CASE "content_block_start":
        IF block.Type == "text":
          display text start
        IF block.Type == "tool_use":
          state = ToolUse
          display "Tool: {block.Name}({block.Input})..."

      CASE "content_block_delta":
        IF block.Type == "text_delta":
          print text incrementally
        IF block.Type == "input_json_delta":
          accumulate tool input

      CASE "content_block_stop":
        IF state == ToolUse:
          state = Streaming

      CASE "message_delta":
        // contains stop_reason and usage delta

      CASE "message_stop":
        state = Done

    // 5. HANDLE TOOL CALLS
    FOR EACH tool_use block in response:
      // Permission check
      IF !permissionManager.CheckPermission(tool):
        append denied tool_result
        continue

      // Pre-tool hooks
      hookResult = hookRunner.RunPreToolUse(tool.Name, tool.Input)
      IF hookResult.Denied:
        append denied tool_result with denial message
        continue

      // Execute tool
      output = toolExecutor.ExecuteTool(tool.Name, tool.Input)

      // Post-tool hooks
      hookRunner.RunPostToolUse(tool.Name, tool.Input, output)

      // Append result
      append tool_result to session

      // Special handling for file edits
      IF tool.Name == "edit_file" or "write_file":
        display diff to user
        ask for approval if in prompt mode

    // 6. IF tool_use was stop_reason:
    //    Continue loop (send tool results back to API)
    IF stopReason == "tool_use":
      continue  // next iteration sends tool results

    // 7. DISPLAY USAGE
    display_usage_summary(usageTracker)

    // 8. SAVE SESSION
    save_session(session)
```

---

## Terminal Input (Modal Line Editor)

### EditorMode

```go
type EditorMode int
// Normal | Insert | Visual | Command
```

### Key Bindings (Normal mode)

| Key | Action |
|-----|--------|
| h | Move cursor left |
| l | Move cursor right |
| j | Move cursor down (multiline) |
| k | Move cursor up (multiline) |
| i | Enter Insert mode |
| a | Enter Insert mode (after cursor) |
| A | Move to end of line, enter Insert mode |
| o | Open new line below, enter Insert mode |
| O | Open new line above, enter Insert mode |
| dd | Delete entire line |
| yy | Yank (copy) entire line |
| p | Paste after cursor |
| x | Delete character under cursor |
| 0 | Move to beginning of line |
| $ | Move to end of line |
| w | Move to next word |
| b | Move to previous word |
| G | Move to end of input |
| gg | Move to start of input |

### Key Bindings (Insert mode)

| Key | Action |
|-----|--------|
| Escape | Return to Normal mode |
| Enter | Submit input (if non-empty) |
| Backspace | Delete character before cursor |
| Delete | Delete character under cursor |
| Left/Right | Move cursor |
| Up/Down | History navigation |
| Tab | Autocomplete (commands/filenames) |

### Key Bindings (Command mode, entered with :)

| Command | Action |
|---------|--------|
| :w | No-op (compatibility) |
| :q | Quit REPL |
| :wq | Quit REPL |

### Implementation

```
1. Enter raw terminal mode (disable canonical input, echo)
2. Read keystrokes as raw bytes
3. Parse escape sequences for special keys
4. Maintain buffer + cursor position
5. Render buffer to terminal on each change
6. Handle multiline input with wrapping
7. Support history (up/down arrows cycle through previous inputs)
```

---

## Terminal Rendering

### Markdown to ANSI Conversion

```
render_markdown(text):
  1. Parse markdown into blocks:
     - Headings (## style)
     - Code blocks (``` with language)
     - Inline code
     - Bold, italic
     - Lists (ordered, unordered)
     - Tables
     - Links
     - Plain text

  2. For each block:
     Headings: apply bold + color (e.g. cyan)
     Code blocks: syntax highlight using language grammar
     Inline code: wrap in backticks, apply dim color
     Bold: apply ANSI bold
     Italic: apply ANSI italic (if terminal supports)
     Lists: proper indentation with bullets/numbers
     Tables: aligned columns with borders
     Links: format as underlined text (URL)

  3. Return ANSI-formatted string
```

### Syntax Highlighting

Uses syntect in Rust. In Go, use Chroma:
- Detect language from code block fence
- Apply theme (defaults to terminal-friendly theme)
- Render highlighted tokens with ANSI color codes

### Streaming Rendering

```
render_stream(delta):
  Maintain state across calls:
  - current_block: none | text | code_block
  - code_language: string
  - code_buffer: string

  FOR each character in delta:
    IF inside code fence:
      IF closing fence detected:
        Flush code buffer with syntax highlighting
        current_block = none
      ELSE:
        Append to code_buffer
    ELSE:
      IF opening fence detected:
        Flush text buffer with markdown rendering
        Extract language from fence
        current_block = code_block
      ELSE:
        Append to text buffer
        IF newline: flush rendered text
```

---

## Usage Display

```
display_usage_summary(tracker):
  input_cost = tracker.Cumulative.InputTokens * pricing.InputCostPerMillion / 1_000_000
  output_cost = tracker.Cumulative.OutputTokens * pricing.OutputCostPerMillion / 1_000_000
  cache_create_cost = tracker.Cumulative.CacheCreationInputTokens * pricing.CacheCreationCostPerMillion / 1_000_000
  cache_read_cost = tracker.Cumulative.CacheReadInputTokens * pricing.CacheReadCostPerMillion / 1_000_000

  total = input_cost + output_cost + cache_create_cost + cache_read_cost

  Print:
    "Tokens: {input}in + {output}out ({cache_create} cache create, {cache_read} cache read)"
    "Cost: ${total:.4f}"
```
