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
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	buf := make([]byte, 1)
	var line strings.Builder
	var suggestions []commands.Spec
	selectedIdx := 0

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
			clearCompletion(len(suggestions))
			fmt.Print("\r\n")
			return "", fmt.Errorf("interrupted")

		case b == 13 || b == 10: // Enter
			clearCompletion(len(suggestions))
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
				// Redraw
				clearCompletion(len(suggestions))
				suggestions = nil
				redrawLine(prompt, line.String())
				// Re-trigger completion if still starts with /
				if strings.HasPrefix(line.String(), "/") && !strings.Contains(line.String()[1:], " ") {
					suggestions = matchCommands(line.String())
					selectedIdx = 0
					if len(suggestions) > 0 {
						showCompletion(suggestions, selectedIdx)
					}
				}
			}

		case b == 9: // Tab
			if len(suggestions) > 0 {
				clearCompletion(len(suggestions))
				// If multiple suggestions, cycle through them
				if len(suggestions) > 1 {
					selectedIdx = (selectedIdx + 1) % len(suggestions)
					line.Reset()
					line.WriteString("/" + suggestions[selectedIdx].Name)
				} else {
					// Single match - complete it
					line.Reset()
					line.WriteString("/" + suggestions[0].Name)
				}
				redrawLine(prompt, line.String())
				// Re-show suggestions for multiple matches
				if len(suggestions) > 1 {
					showCompletion(suggestions, selectedIdx)
				} else {
					suggestions = nil
				}
			}

		default:
			// Printable character
			if b >= 32 && b < 127 {
				clearCompletion(len(suggestions))
				suggestions = nil

				line.WriteByte(b)
				fmt.Print(string(b))

				// Show completions when typing a slash command
				current := line.String()
				if strings.HasPrefix(current, "/") && !strings.Contains(current[1:], " ") {
					suggestions = matchCommands(current)
					selectedIdx = 0
					if len(suggestions) > 0 {
						showCompletion(suggestions, selectedIdx)
					}
				}
			}
		}
	}
}

// redrawLine clears the current line and redraws prompt + content.
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

// showCompletion renders the autocomplete suggestions below the current line.
func showCompletion(specs []commands.Spec, selected int) {
	fmt.Print("\r\n")
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
		fmt.Printf("%s%s/%s%s - %s%s\r\n", icon, style, spec.Name, hint+aliases, spec.Summary, Reset)
	}
}

// clearCompletion erases the completion suggestions from the terminal.
func clearCompletion(count int) {
	if count == 0 {
		return
	}
	EraseLines(count)
}
