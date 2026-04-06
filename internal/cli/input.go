package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/hieu-glaw/glaw-code/internal/commands"
	"golang.org/x/term"
)

// ReadLineWithCompletion reads a line from stdin with slash-command autocomplete.
// When the user types '/', matching commands are shown. Tab completes the prefix.
func ReadLineWithCompletion(prompt string) (string, error) {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		// Fallback to simple line reading if raw mode fails
		fmt.Print(prompt)
		var line string
		_, err := fmt.Fscanln(os.Stdin, &line)
		return line, err
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	buf := make([]byte, 1)
	var line strings.Builder
	selectedIdx := 0
	completionLineCount := 0

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
			return "", fmt.Errorf("interrupted")

		case b == 13 || b == 10: // Enter
			clearAndEraseCompletion(&completionLineCount)
			fmt.Print("\r\n")
			return line.String(), nil

		case b == 127 || b == 8: // Backspace
			if line.Len() > 0 {
				// Remove last rune
				str := line.String()
				runes := []rune(str)
				runes = runes[:len(runes)-1]
				line.Reset()
				line.WriteString(string(runes))
				clearAndEraseCompletion(&completionLineCount)
				redrawLine(prompt, line.String())
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
					redrawLine(prompt, line.String())
					if len(matches) > 1 {
						completionLineCount = showCompletion(matches, selectedIdx)
					}
				}
			}

		default:
			// Printable character
			if b >= 32 && b < 127 {
				clearAndEraseCompletion(&completionLineCount)

				line.WriteByte(b)
				redrawLine(prompt, line.String())

				completionLineCount = tryShowCompletion(line.String(), &selectedIdx)
			}
		}
	}
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

// redrawLine clears the current line and redraws prompt + content.
// After calling this the cursor is positioned after the content.
func redrawLine(prompt, content string) {
	fmt.Print("\r\033[K" + prompt + content)
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

	// Build all the lines first so we know exactly how many we print.
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

	// Cursor is at end of input line. Move down and print each suggestion.
	for _, l := range lines {
		fmt.Print("\r\n" + l)
	}
	totalLines := len(lines)

	// Move cursor back up to the input line.
	fmt.Printf("\033[%dA", totalLines)

	// Now cursor is at column 0 of the input line.
	// We need it at the end of the prompt+content so the next typed
	// character appears in the right place. Move right by a large number —
	// the terminal clamps it to the actual end of line content.
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
	// Move cursor down to the last suggestion line
	fmt.Printf("\033[%dB", lineCount)
	// Erase all lines moving back up to (and including) the first suggestion line
	for i := 0; i < lineCount; i++ {
		ClearLine()
		MoveUp(1)
	}
	// One final clear for the line we landed on
	ClearLine()
	// Cursor is now at column 0 of the input line
}
