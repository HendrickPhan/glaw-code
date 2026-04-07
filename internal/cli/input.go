package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hieu-glaw/glaw-code/internal/commands"
	"golang.org/x/term"
)

// ErrInterrupted is returned when the user presses Ctrl+C at the input prompt.
var ErrInterrupted = errors.New("interrupted")

// InputHistory manages command history with navigation.
type InputHistory struct {
	entries []string
	pos     int // current position (0 = newest, len-1 = oldest)
}

// NewInputHistory creates a new input history.
func NewInputHistory() *InputHistory {
	return &InputHistory{
		pos: -1, // -1 means not navigating
	}
}

// Add adds an entry to the history and resets the navigation position.
func (h *InputHistory) Add(entry string) {
	if entry == "" {
		return
	}
	// Don't add duplicates of the most recent entry
	if len(h.entries) > 0 && h.entries[0] == entry {
		h.pos = -1
		return
	}
	// Prepend to front
	h.entries = append([]string{entry}, h.entries...)
	// Limit history size
	if len(h.entries) > 1000 {
		h.entries = h.entries[:1000]
	}
	h.pos = -1
}

// Older moves to an older entry and returns it. Returns ("", false) if at the end.
func (h *InputHistory) Older() (string, bool) {
	if len(h.entries) == 0 {
		return "", false
	}
	if h.pos < len(h.entries)-1 {
		h.pos++
	}
	if h.pos >= 0 && h.pos < len(h.entries) {
		return h.entries[h.pos], true
	}
	return "", false
}

// Newer moves to a newer entry and returns it. Returns ("", false) if at the newest.
func (h *InputHistory) Newer() (string, bool) {
	if h.pos > 0 {
		h.pos--
		return h.entries[h.pos], true
	}
	h.pos = -1
	return "", false
}

// Reset resets the navigation position.
func (h *InputHistory) Reset() {
	h.pos = -1
}

// ReadLineWithCompletion reads a line from stdin with slash-command autocomplete,
// command history (up/down arrows), and paste detection (bracket paste mode).
func ReadLineWithCompletion(prompt string) (string, error) {
	return ReadLineWithCompletionAndHistory(prompt, nil)
}

// ReadLineWithCompletionAndHistory reads a line with completion and history support.
func ReadLineWithCompletionAndHistory(prompt string, history *InputHistory) (string, error) {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		// Fallback to simple line reading if raw mode fails
		fmt.Print(prompt)
		var line string
		_, err := fmt.Fscanln(os.Stdin, &line)
		return line, err
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	// Enable bracket paste mode so we can detect paste events
	fmt.Print("\x1b[?2004h")
	defer fmt.Print("\x1b[?2004l")

	buf := make([]byte, 1)
	var line strings.Builder
	selectedIdx := 0
	completionLineCount := 0

	// Track paste state
	pasting := false
	var pasteBuf strings.Builder

	// Track saved line before history navigation
	savedLine := ""
	navigatingHistory := false

	// Track previous display line count for proper multi-line redraw
	prevLineCount := 1

	// Print prompt
	fmt.Print(prompt)

	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			if line.Len() > 0 {
				return line.String(), nil
			}
			return "", err
		}

		b := buf[0]

		switch {
		case b == 3: // Ctrl+C
			clearAndEraseCompletion(&completionLineCount)
			fmt.Print("\r\n")
			return "", ErrInterrupted

		case b == 13 || b == 10: // Enter (CR or LF)
			// If we're in a paste, ignore Enter - it's part of the pasted content
			if pasting {
				pasteBuf.WriteByte('\n')
				continue
			}
			clearAndEraseCompletion(&completionLineCount)
			fmt.Print("\r\n")
			result := line.String()
			if history != nil {
				history.Add(result)
			}
			return result, nil

		case b == 15: // Ctrl+O - signal to caller (thinking detail toggle)
			clearAndEraseCompletion(&completionLineCount)
			return "\x0f", nil

		case b == 27: // ESC - start of escape sequence
			seq := make([]byte, 0, 4)
			seq = append(seq, b)

			// Try to read the '[' character
			if n, err := os.Stdin.Read(buf); err != nil || n == 0 {
				continue
			}
			if buf[0] != '[' && buf[0] != 'O' {
				continue
			}
			seq = append(seq, buf[0])

			// Read the command byte
			if n, err := os.Stdin.Read(buf); err != nil || n == 0 {
				continue
			}
			seq = append(seq, buf[0])

			// Check for bracket paste: ESC[200~ (paste start) and ESC[201~ (paste end)
			if seq[1] == '[' && buf[0] == '2' {
				// Read 3 more bytes: digit, digit, '~'
				pasteSeq := make([]byte, 3)
				if n, err := os.Stdin.Read(pasteSeq); err != nil || n < 3 {
					continue
				}
				if pasteSeq[0] == '0' && pasteSeq[1] == '0' && pasteSeq[2] == '~' {
					// ESC[200~ - paste start
					pasting = true
					pasteBuf.Reset()
					continue
				}
				if pasteSeq[0] == '0' && pasteSeq[1] == '1' && pasteSeq[2] == '~' {
					// ESC[201~ - paste end
					pasting = false
					pasted := cleanPastedContent(pasteBuf.String())
					if pasted != "" {
						clearAndEraseCompletion(&completionLineCount)
						line.WriteString(pasted)
						prevLineCount = redrawLine(prompt, line.String(), prevLineCount)
						completionLineCount = tryShowCompletion(line.String(), &selectedIdx)
					}
					continue
				}
				continue
			}

			// Arrow keys and other escape sequences
			switch buf[0] {
			case 'A': // Up arrow - history navigation
				if history != nil {
					clearAndEraseCompletion(&completionLineCount)
					if !navigatingHistory {
						savedLine = line.String()
						navigatingHistory = true
					}
					if entry, ok := history.Older(); ok {
						line.Reset()
						line.WriteString(entry)
						prevLineCount = redrawLine(prompt, line.String(), prevLineCount)
					}
				}
			case 'B': // Down arrow - history navigation
				if history != nil {
					clearAndEraseCompletion(&completionLineCount)
					if navigatingHistory {
						if entry, ok := history.Newer(); ok {
							line.Reset()
							line.WriteString(entry)
							prevLineCount = redrawLine(prompt, line.String(), prevLineCount)
						} else {
							line.Reset()
							line.WriteString(savedLine)
							navigatingHistory = false
							prevLineCount = redrawLine(prompt, line.String(), prevLineCount)
						}
					}
				}
			case 'C': // Right arrow - ignore
			case 'D': // Left arrow - ignore
			default:
				// Handle extended sequences like ESC[1;5D (Ctrl+Left), etc.
				if buf[0] == '1' {
					extBuf := make([]byte, 2)
					if n, err := os.Stdin.Read(extBuf); err == nil && n == 2 {
						// e.g., ESC[1;5C = Ctrl+Right, ESC[1;5D = Ctrl+Left
					}
				}
			}
			continue

		case b == 127 || b == 8: // Backspace
			if line.Len() > 0 {
				str := line.String()
				runes := []rune(str)
				runes = runes[:len(runes)-1]
				line.Reset()
				line.WriteString(string(runes))
				clearAndEraseCompletion(&completionLineCount)
				prevLineCount = redrawLine(prompt, line.String(), prevLineCount)
				completionLineCount = tryShowCompletion(line.String(), &selectedIdx)
			}

		case b == 9: // Tab
			if completionLineCount > 0 {
				matches := matchCommands(line.String())
				if len(matches) > 0 {
					clearAndEraseCompletion(&completionLineCount)
					if len(matches) > 1 {
						selectedIdx = (selectedIdx + 1) % len(matches)
						line.Reset()
						line.WriteString("/" + matches[selectedIdx].Name)
					} else {
						line.Reset()
						line.WriteString("/" + matches[0].Name)
					}
					prevLineCount = redrawLine(prompt, line.String(), prevLineCount)
					if len(matches) > 1 {
						completionLineCount = showCompletion(matches, selectedIdx)
					}
				}
			}

		default:
			// Handle pasting (when terminal doesn't support bracket paste mode)
			if pasting {
				if b >= 32 && b < 127 {
					pasteBuf.WriteByte(b)
				}
				continue
			}
			// Printable character
			if b >= 32 && b < 127 {
				if navigatingHistory {
					navigatingHistory = false
				}
				clearAndEraseCompletion(&completionLineCount)
				line.WriteByte(b)
				prevLineCount = redrawLine(prompt, line.String(), prevLineCount)
				completionLineCount = tryShowCompletion(line.String(), &selectedIdx)
			}
		}
	}
}

// ReadLineDuringAction reads a line from stdin while an action is running.
func ReadLineDuringAction(prompt string, history *InputHistory) (string, error) {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	// Enable bracket paste mode
	fmt.Print("\x1b[?2004h")
	defer fmt.Print("\x1b[?2004l")

	buf := make([]byte, 1)
	var line strings.Builder
	selectedIdx := 0
	completionLineCount := 0
	pasting := false
	var pasteBuf strings.Builder
	prevLineCount := 1

	fmt.Print(prompt)

	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			if line.Len() > 0 {
				return line.String(), nil
			}
			return "", err
		}

		b := buf[0]

		switch {
		case b == 3: // Ctrl+C
			clearAndEraseCompletion(&completionLineCount)
			fmt.Print("\r\n")
			return "", ErrInterrupted

		case b == 13 || b == 10: // Enter
			if pasting {
				pasteBuf.WriteByte('\n')
				continue
			}
			clearAndEraseCompletion(&completionLineCount)
			fmt.Print("\r\n")
			result := line.String()
			if history != nil {
				history.Add(result)
			}
			return result, nil

		case b == 15: // Ctrl+O
			clearAndEraseCompletion(&completionLineCount)
			return "\x0f", nil

		case b == 27: // ESC sequence
			seq := make([]byte, 0, 4)
			seq = append(seq, b)

			if n, err := os.Stdin.Read(buf); err != nil || n == 0 {
				continue
			}
			if buf[0] != '[' && buf[0] != 'O' {
				continue
			}
			seq = append(seq, buf[0])

			if n, err := os.Stdin.Read(buf); err != nil || n == 0 {
				continue
			}
			seq = append(seq, buf[0])

			// Bracket paste detection
			if seq[1] == '[' && buf[0] == '2' {
				pasteSeq := make([]byte, 3)
				if n, err := os.Stdin.Read(pasteSeq); err != nil || n < 3 {
					continue
				}
				if pasteSeq[0] == '0' && pasteSeq[1] == '0' && pasteSeq[2] == '~' {
					pasting = true
					pasteBuf.Reset()
					continue
				}
				if pasteSeq[0] == '0' && pasteSeq[1] == '1' && pasteSeq[2] == '~' {
					pasting = false
					pasted := cleanPastedContent(pasteBuf.String())
					if pasted != "" {
						clearAndEraseCompletion(&completionLineCount)
						line.WriteString(pasted)
						prevLineCount = redrawLine(prompt, line.String(), prevLineCount)
						completionLineCount = tryShowCompletion(line.String(), &selectedIdx)
					}
					continue
				}
				continue
			}

			// Arrow keys for history
			switch buf[0] {
			case 'A': // Up arrow
				if history != nil {
					clearAndEraseCompletion(&completionLineCount)
					if entry, ok := history.Older(); ok {
						line.Reset()
						line.WriteString(entry)
						prevLineCount = redrawLine(prompt, line.String(), prevLineCount)
					}
				}
			case 'B': // Down arrow
				if history != nil {
					clearAndEraseCompletion(&completionLineCount)
					if entry, ok := history.Newer(); ok {
						line.Reset()
						line.WriteString(entry)
						prevLineCount = redrawLine(prompt, line.String(), prevLineCount)
					} else {
						line.Reset()
						prevLineCount = redrawLine(prompt, line.String(), prevLineCount)
					}
				}
			}
			continue

		case b == 127 || b == 8: // Backspace
			if line.Len() > 0 {
				str := line.String()
				runes := []rune(str)
				runes = runes[:len(runes)-1]
				line.Reset()
				line.WriteString(string(runes))
				clearAndEraseCompletion(&completionLineCount)
				prevLineCount = redrawLine(prompt, line.String(), prevLineCount)
				completionLineCount = tryShowCompletion(line.String(), &selectedIdx)
			}

		case b == 9: // Tab
			if completionLineCount > 0 {
				matches := matchCommands(line.String())
				if len(matches) > 0 {
					clearAndEraseCompletion(&completionLineCount)
					if len(matches) > 1 {
						selectedIdx = (selectedIdx + 1) % len(matches)
						line.Reset()
						line.WriteString("/" + matches[selectedIdx].Name)
					} else {
						line.Reset()
						line.WriteString("/" + matches[0].Name)
					}
					prevLineCount = redrawLine(prompt, line.String(), prevLineCount)
					if len(matches) > 1 {
						completionLineCount = showCompletion(matches, selectedIdx)
					}
				}
			}

		default:
			if pasting {
				if b >= 32 && b < 127 {
					pasteBuf.WriteByte(b)
				}
				continue
			}
			if b >= 32 && b < 127 {
				clearAndEraseCompletion(&completionLineCount)
				line.WriteByte(b)
				prevLineCount = redrawLine(prompt, line.String(), prevLineCount)
				completionLineCount = tryShowCompletion(line.String(), &selectedIdx)
			}
		}
	}
}

// WaitForInputOrSignal reads stdin non-blocking, returning any input line or
// empty string if interrupted by the done channel before a full line was read.
func WaitForInputOrSignal(prompt string, done <-chan struct{}) string {
	ch := make(chan string, 1)
	go func() {
		oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			return
		}
		defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

		fmt.Print(prompt)

		buf := make([]byte, 1)
		var line strings.Builder

		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				if line.Len() > 0 {
					ch <- line.String()
				}
				ch <- ""
				return
			}

			b := buf[0]
			switch {
			case b == 13 || b == 10: // Enter
				fmt.Print("\r\n")
				ch <- line.String()
				return
			case b == 3: // Ctrl+C
				fmt.Print("\r\n")
				ch <- ""
				return
			case b == 127 || b == 8: // Backspace
				if line.Len() > 0 {
					str := line.String()
					runes := []rune(str)
					runes = runes[:len(runes)-1]
					line.Reset()
					line.WriteString(string(runes))
					fmt.Print("\r\033[K" + prompt + line.String())
				}
			default:
				if b >= 32 && b < 127 {
					line.WriteByte(b)
					fmt.Print(string(b))
				}
			}
		}
	}()

	select {
	case line := <-ch:
		return line
	case <-done:
		return ""
	case <-time.After(50 * time.Millisecond):
		return ""
	}
}

// cleanPastedContent cleans up pasted content: removes carriage returns,
// preserves newlines for multi-line paste, and strips other control characters.
func cleanPastedContent(pasted string) string {
	pasted = strings.Map(func(r rune) rune {
		if r == '\r' {
			return -1 // Remove carriage returns
		}
		if r < 32 && r != '\n' && r != '\t' {
			return -1 // Strip other control chars but keep newlines and tabs
		}
		return r
	}, pasted)
	pasted = strings.TrimSpace(pasted)
	return pasted
}

// clearAndEraseCompletion erases completion suggestions and resets the counter to 0.
func clearAndEraseCompletion(count *int) {
	if *count > 0 {
		eraseCompletion(*count)
	}
	*count = 0
}

// tryShowCompletion shows autocomplete suggestions if the input is a slash command prefix.
// Returns the number of completion lines printed.
func tryShowCompletion(input string, selectedIdx *int) int {
	if strings.HasPrefix(input, "/") && !strings.Contains(input[1:], " ") {
		matches := matchCommands(input)
		*selectedIdx = 0
		return showCompletion(matches, *selectedIdx)
	}
	return 0
}

// redrawLine clears the current line(s) and redraws prompt + content.
// prevLines is the number of lines previously displayed (1 = single line).
// Returns the new number of lines occupied by the content.
func redrawLine(prompt, content string, prevLines int) int {
	// Move up to clear previous multi-line content
	for i := 0; i < prevLines-1; i++ {
		fmt.Print("\033[A") // move up
	}
	// Clear from cursor to end of screen (handles all lines below)
	fmt.Print("\r\033[J")
	// Draw the new content
	fmt.Print(prompt + content)
	// Return new line count
	return strings.Count(content, "\n") + 1
}

// matchCommands returns commands whose name or alias starts with the input.
func matchCommands(input string) []commands.Spec {
	input = strings.TrimPrefix(input, "/")
	if input == "" {
		return commands.Specs
	}

	var matches []commands.Spec
	for _, spec := range commands.Specs {
		if strings.HasPrefix(spec.Name, input) {
			matches = append(matches, spec)
			continue
		}
		for _, alias := range spec.Aliases {
			if strings.HasPrefix(alias, input) {
				matches = append(matches, spec)
				break
			}
		}
	}
	return matches
}

// showCompletion renders autocomplete suggestions below the current input line.
// The cursor must be at the end of the input line when calling this.
// After drawing, the cursor is returned to the end of the input line.
// Returns the number of lines printed below the input line.
func showCompletion(specs []commands.Spec, selected int) int {
	if len(specs) == 0 {
		return 0
	}

	var lines []string
	for i, spec := range specs {
		style := Dim
		if i == selected {
			style = Bold + Cyan
		}
		aliases := ""
		if len(spec.Aliases) > 0 {
			aliases = fmt.Sprintf(" (%s)", strings.Join(spec.Aliases, ", "))
		}
		hint := ""
		if spec.ArgumentHint != "" {
			hint = " " + spec.ArgumentHint
		}
		icon := "  "
		if i == selected {
			icon = "> "
		}
		lines = append(lines, fmt.Sprintf("%s%s/%s%s - %s%s", icon, style, spec.Name, hint+aliases, spec.Summary, Reset))
	}

	for _, l := range lines {
		fmt.Print("\r\n" + l)
	}
	totalLines := len(lines)

	fmt.Printf("\033[%dA", totalLines)
	fmt.Print("\033[999C")

	return totalLines
}

// eraseCompletion erases completion suggestions that were drawn below the input line.
// The cursor must be at the end of the input line when calling this.
// After erasing, the cursor is at the start of the line (column 0) so the caller
// should call redrawLine to repaint the input.
func eraseCompletion(lineCount int) {
	if lineCount == 0 {
		return
	}
	fmt.Printf("\033[%dB", lineCount)
	for i := 0; i < lineCount; i++ {
		ClearLine()
		MoveUp(1)
	}
	ClearLine()
}
