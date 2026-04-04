package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/chroma/v2"
	chromaLexers "github.com/alecthomas/chroma/v2/lexers"
	chromaStyles "github.com/alecthomas/chroma/v2/styles"
)

// ANSI color codes
const (
	Reset   = "\033[0m"
	Bold    = "\033[1m"
	Dim     = "\033[2m"
	Cyan    = "\033[36m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Red     = "\033[31m"
	Magenta = "\033[35m"
	Blue    = "\033[34m"
	White   = "\033[37m"
	Italic  = "\033[3m"
)

// SpinnerFrames are the animation frames for the loading spinner.
var SpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// ClearLine clears the current terminal line and moves cursor to start.
func ClearLine() {
	fmt.Print("\r\033[K")
}

// MoveUp moves the cursor up n lines.
func MoveUp(n int) {
	fmt.Printf("\033[%dA", n)
}

// MoveDown moves the cursor down n lines.
func MoveDown(n int) {
	fmt.Printf("\033[%dB", n)
}

// EraseLines erases n lines from the current position upward.
func EraseLines(n int) {
	for i := 0; i < n; i++ {
		MoveUp(1)
		ClearLine()
	}
}

// supportsColor is set based on terminal capability.
var supportsColor bool

func init() {
	term := os.Getenv("TERM")
	supportsColor = term != "" && term != "dumb"
	if os.Getenv("NO_COLOR") != "" {
		supportsColor = false
	}
}

// Regex patterns for markdown parsing
var (
	headingRe       = regexp.MustCompile(`(?m)^(#{1,6})\s+(.+)$`)
	codeBlockRe     = regexp.MustCompile("(?s)```(\\w*)\\n(.*?)```")
	inlineCodeRe    = regexp.MustCompile("`([^`]+)`")
	boldRe          = regexp.MustCompile(`\*\*(.+?)\*\*`)
	italicRe        = regexp.MustCompile(`(?:^|\W)\*([^*]+?)\*(?:\W|$)`)
	linkRe          = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	unorderedListRe = regexp.MustCompile(`(?m)^(\s*)[-*+]\s+(.+)$`)
	orderedListRe   = regexp.MustCompile(`(?m)^(\s*)(\d+\.)\s+(.+)$`)
	hrRe            = regexp.MustCompile(`(?m)^-{3,}$|^_{3,}$|^\*{3,}$`)
	blockquoteRe    = regexp.MustCompile(`(?m)^>\s+(.+)$`)
)

// RenderMarkdown converts markdown text to ANSI-colored terminal output.
// Handles: headings, code blocks, inline code, bold, italic, links, lists,
// blockquotes, horizontal rules, and syntax-highlighted code blocks.
func RenderMarkdown(text string) string {
	if !supportsColor {
		return text
	}

	// Process fenced code blocks first (they contain markdown that shouldn't be processed)
	text = codeBlockRe.ReplaceAllStringFunc(text, func(match string) string {
		sub := codeBlockRe.FindStringSubmatch(match)
		lang := sub[1]
		code := sub[2]

		var sb strings.Builder
		sb.WriteString(Dim + "┌───")
		if lang != "" {
			sb.WriteString(" " + lang)
		}
		sb.WriteString("\n" + Reset)

		code = strings.TrimSuffix(code, "\n")

		if lang != "" {
			highlighted := highlightCode(code, lang)
			for _, line := range strings.Split(highlighted, "\n") {
				sb.WriteString(Dim + "│" + Reset + " " + line + "\n")
			}
		} else {
			for _, line := range strings.Split(code, "\n") {
				sb.WriteString(Dim + "│" + Reset + " " + line + "\n")
			}
		}

		sb.WriteString(Dim + "└───" + Reset)
		return sb.String()
	})

	// Horizontal rules
	text = hrRe.ReplaceAllString(text, Dim+"────────────────────────────────\n"+Reset)

	// Headings
	text = headingRe.ReplaceAllStringFunc(text, func(match string) string {
		sub := headingRe.FindStringSubmatch(match)
		level := len(sub[1])
		headingText := sub[2]
		switch level {
		case 1:
			return Bold + Cyan + "═══ " + headingText + " ═══" + Reset
		case 2:
			return Bold + Blue + "  ■ " + headingText + Reset
		case 3:
			return Bold + Magenta + "    ▸ " + headingText + Reset
		default:
			return Bold + "    · " + headingText + Reset
		}
	})

	// Bold
	text = boldRe.ReplaceAllString(text, Bold+"$1"+Reset)

	// Italic
	text = italicRe.ReplaceAllString(text, Italic+"$1"+Reset)

	// Inline code
	text = inlineCodeRe.ReplaceAllString(text, Yellow+"`$1`"+Reset)

	// Links
	text = linkRe.ReplaceAllString(text, Cyan+"$1"+Dim+" ($2)"+Reset)

	// Unordered lists
	text = unorderedListRe.ReplaceAllString(text, Green+"  •"+Reset+" $2")

	// Ordered lists
	text = orderedListRe.ReplaceAllString(text, Yellow+"  $2"+Reset+" $3")

	// Blockquotes
	text = blockquoteRe.ReplaceAllString(text, Dim+"│ "+Reset+"$1")

	return text
}

// highlightCode applies syntax highlighting using Chroma.
func highlightCode(code, language string) string {
	lexer := chroma.Coalesce(chromaLexers.Get(language))
	if lexer == nil {
		return code
	}

	style := chromaStyles.Get("monokai")
	if style == nil {
		style = chromaStyles.Fallback
	}

	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return code
	}

	var buf strings.Builder
	for token := iterator(); token != chroma.EOF; token = iterator() {
		entry := style.Get(token.Type)
		ansi := styleEntryToANSI(entry)
		if ansi != "" {
			buf.WriteString(ansi + token.String() + Reset)
		} else {
			buf.WriteString(token.String())
		}
	}
	return buf.String()
}

func styleEntryToANSI(entry chroma.StyleEntry) string {
	var codes []string
	if entry.Bold == chroma.Yes {
		codes = append(codes, Bold)
	}
	if entry.Italic == chroma.Yes {
		codes = append(codes, Italic)
	}
	if entry.Underline == chroma.Yes {
		codes = append(codes, "\033[4m")
	}
	if entry.Colour.IsSet() {
		r, g, b := entry.Colour.Red(), entry.Colour.Green(), entry.Colour.Blue()
		codes = append(codes, fmt.Sprintf("\033[38;2;%d;%d;%dm", r, g, b))
	}
	return strings.Join(codes, "")
}

// RenderCodeBlock renders a code block with optional syntax highlighting.
func RenderCodeBlock(code, language string) string {
	var sb strings.Builder
	sb.WriteString(Dim + "┌───")
	if language != "" {
		sb.WriteString(" " + language)
	}
	sb.WriteString("\n")

	if language != "" && supportsColor {
		highlighted := highlightCode(code, language)
		for _, line := range strings.Split(highlighted, "\n") {
			sb.WriteString(Dim + "│" + Reset + " " + line + "\n")
		}
	} else {
		for _, line := range strings.Split(code, "\n") {
			sb.WriteString(Dim + "│" + Reset + " " + line + "\n")
		}
	}

	sb.WriteString(Dim + "└───" + Reset)
	return sb.String()
}

// RenderToolCall formats a tool call for display.
func RenderToolCall(name string, input string) string {
	display := input
	if len(display) > 200 {
		display = display[:200] + "..."
	}
	return fmt.Sprintf("%s⚙ %s%s%s %s%s%s",
		Yellow, Bold, name, Reset,
		Dim, display, Reset)
}

// RenderToolResult formats a tool result for display.
func RenderToolResult(content string, isError bool) string {
	if isError {
		return fmt.Sprintf("%s✗ Error:%s %s", Red, Reset, content)
	}
	display := content
	if len(display) > 500 {
		display = display[:500] + "..."
	}
	return fmt.Sprintf("%s✓%s %s", Green, Reset, display)
}

// RenderUsage displays token usage and cost.
func RenderUsage(inputTokens, outputTokens int, cost float64) string {
	return fmt.Sprintf("%s📊 Tokens:%s %din + %dout %s│ Cost:%s $%.4f",
		Dim, Reset, inputTokens, outputTokens,
		Dim, Reset, cost)
}

// ProgressBar renders a simple progress indicator.
func ProgressBar(label string, current, total int) string {
	width := 30
	pct := float64(current) / float64(total)
	filled := int(pct * float64(width))

	var bar strings.Builder
	bar.WriteString(fmt.Sprintf("%s ", label))
	for i := 0; i < width; i++ {
		if i < filled {
			bar.WriteString("█")
		} else {
			bar.WriteString("░")
		}
	}
	bar.WriteString(fmt.Sprintf(" %d/%d (%.0f%%)", current, total, pct*100))
	return bar.String()
}

// DetectLanguage detects programming language from file extension.
func DetectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".rb":
		return "ruby"
	case ".sh":
		return "bash"
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".md":
		return "markdown"
	case ".html":
		return "html"
	case ".css":
		return "css"
	case ".sql":
		return "sql"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".cxx":
		return "cpp"
	default:
		return ""
	}
}

// Spinner displays an animated loading indicator in the terminal.
type Spinner struct {
	label   string
	frame   int
	stop    chan struct{}
	stopped chan struct{}
	mu      sync.Mutex
	done    bool
}

// NewSpinner creates and starts a spinner with the given label.
func NewSpinner(label string) *Spinner {
	s := &Spinner{
		label:   label,
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
	go s.run()
	return s
}

// Update changes the spinner label text.
func (s *Spinner) Update(label string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.label = label
}

func (s *Spinner) run() {
	defer close(s.stopped)
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.stop:
			ClearLine()
			return
		case <-ticker.C:
			s.mu.Lock()
			label := s.label
			frame := SpinnerFrames[s.frame%len(SpinnerFrames)]
			s.frame++
			s.mu.Unlock()
			ClearLine()
			fmt.Printf("\r%s%s%s %s", Cyan, frame, Reset, label)
		}
	}
}

// Stop stops the spinner and clears its line.
func (s *Spinner) Stop() {
	s.mu.Lock()
	if s.done {
		s.mu.Unlock()
		return
	}
	s.done = true
	s.mu.Unlock()
	close(s.stop)
	<-s.stopped
}

// RenderThinkingBlock renders a collapsible "thinking" text block.
// Shows the first line as a header; remaining lines are dimmed and can be expanded.
func RenderThinkingBlock(text string, collapsed bool) string {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || text == "" {
		return ""
	}

	var sb strings.Builder
	header := lines[0]
	if len(header) > 80 {
		header = header[:77] + "..."
	}

	if collapsed || len(lines) <= 1 {
		sb.WriteString(fmt.Sprintf("%s%s⏺ %s%s\n", Dim, Italic, header, Reset))
	} else {
		sb.WriteString(fmt.Sprintf("%s%s▼ %s%s\n", Cyan, Italic, header, Reset))
		for _, line := range lines[1:] {
			display := line
			if len(display) > 100 {
				display = display[:97] + "..."
			}
			sb.WriteString(fmt.Sprintf("%s  │ %s%s\n", Dim, display, Reset))
		}
	}
	return sb.String()
}

// toolDisplayInfo extracts a short description from tool input JSON.
func toolDisplayInfo(name string, input json.RawMessage) string {
	switch name {
	case "bash":
		var args struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(input, &args) == nil && args.Command != "" {
			cmd := args.Command
			if len(cmd) > 60 {
				cmd = cmd[:57] + "..."
			}
			return cmd
		}
	case "write_file":
		var args struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(input, &args) == nil {
			return args.Path
		}
	case "edit_file":
		var args struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(input, &args) == nil {
			return args.Path
		}
	case "read_file":
		var args struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(input, &args) == nil {
			return args.Path
		}
	case "glob_search":
		var args struct {
			Pattern string `json:"pattern"`
		}
		if json.Unmarshal(input, &args) == nil {
			return args.Pattern
		}
	case "grep_search":
		var args struct {
			Pattern string `json:"pattern"`
		}
		if json.Unmarshal(input, &args) == nil {
			return args.Pattern
		}
	default:
		if len(input) > 50 {
			return string(input[:47]) + "..."
		}
		return string(input)
	}
	return ""
}

// toolIcon returns an icon for a tool name.
func toolIcon(name string) string {
	switch name {
	case "bash":
		return "$"
	case "write_file", "edit_file":
		return "✎"
	case "read_file", "glob_search", "grep_search":
		return "📄"
	case "web_fetch", "web_search":
		return "🌐"
	default:
		return "⚙"
	}
}

// RenderToolHeader renders the one-line header shown when a tool starts.
func RenderToolHeader(name string, input json.RawMessage) string {
	icon := toolIcon(name)
	info := toolDisplayInfo(name, input)
	if info != "" {
		return fmt.Sprintf("%s%s%s %s%-20s%s %s%s%s",
			Yellow, icon, Reset,
			Bold, name, Reset,
			Dim, info, Reset)
	}
	return fmt.Sprintf("%s%s%s %s%s%s",
		Yellow, icon, Reset,
		Bold, name, Reset)
}

// RenderToolDone renders the completion status after a tool finishes.
func RenderToolDone(name string, output string, isError bool, elapsed time.Duration) string {
	if isError {
		display := output
		if len(display) > 100 {
			display = display[:97] + "..."
		}
		return fmt.Sprintf("%s  ✗ %s%s %s(%s%.0fms)%s %s%s%s",
			Red, name, Reset,
			Dim, Red, elapsed.Seconds()*1000, Reset,
			Red, display, Reset)
	}

	display := output
	if len(display) > 80 {
		display = display[:77] + "..."
	}
	return fmt.Sprintf("%s  ✓ %s%s %s(%.0fms)%s %s%s%s",
		Green, name, Reset,
		Dim, elapsed.Seconds()*1000, Reset,
		Dim, display, Reset)
}
