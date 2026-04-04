package lsp

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ServerConfig defines how to connect to a language server.
type ServerConfig struct {
	Name                string            `json:"name"`
	Command             string            `json:"command"`
	Args                []string          `json:"args"`
	Env                 map[string]string `json:"env"`
	WorkspaceRoot       string            `json:"workspace_root"`
	InitializationOptions json.RawMessage `json:"initialization_options"`
	ExtensionToLanguage map[string]string `json:"extension_to_language"` // ".go" -> "go"
}

// Diagnostic represents a language server diagnostic.
type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"`
	Message  string `json:"message"`
	Source   string `json:"source,omitempty"`
	Code     string `json:"code,omitempty"`
}

// Range represents a range in a text document.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Position represents a position in a text document.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// SymbolLocation represents a symbol location in code.
type SymbolLocation struct {
	Path  string
	Line  int
	Col   int
}

// FileDiagnostics holds diagnostics for a single file.
type FileDiagnostics struct {
	Path        string
	URI         string
	Diagnostics []Diagnostic
}

// WorkspaceDiagnostics aggregates diagnostics across all files.
type WorkspaceDiagnostics struct {
	Files []FileDiagnostics
}

// IsEmpty returns true if there are no diagnostics.
func (d *WorkspaceDiagnostics) IsEmpty() bool {
	return len(d.Files) == 0
}

// TotalDiagnostics returns the total count of diagnostics.
func (d *WorkspaceDiagnostics) TotalDiagnostics() int {
	total := 0
	for _, f := range d.Files {
		total += len(f.Diagnostics)
	}
	return total
}

// ContextEnrichment provides LSP context for the system prompt.
type ContextEnrichment struct {
	FilePath    string
	Diagnostics []Diagnostic
	Definitions []SymbolLocation
	References  []SymbolLocation
}

// IsEmpty returns true if there is no enrichment data.
func (e *ContextEnrichment) IsEmpty() bool {
	return len(e.Diagnostics) == 0 && len(e.Definitions) == 0 && len(e.References) == 0
}

// RenderPromptSection formats the enrichment as a markdown section.
func (e *ContextEnrichment) RenderPromptSection() string {
	if e.IsEmpty() {
		return ""
	}

	result := "## LSP Context\n\n"

	if len(e.Diagnostics) > 0 {
		limit := len(e.Diagnostics)
		if limit > 12 {
			limit = 12
		}
		result += fmt.Sprintf("### Diagnostics (%d)\n", len(e.Diagnostics))
		for i := 0; i < limit; i++ {
			d := e.Diagnostics[i]
			severity := severityString(d.Severity)
			result += fmt.Sprintf("- **%s:%d:%d**: %s [%s]\n",
				e.FilePath, d.Range.Start.Line+1, d.Range.Start.Character+1, d.Message, severity)
		}
		result += "\n"
	}

	if len(e.Definitions) > 0 {
		limit := len(e.Definitions)
		if limit > 12 {
			limit = 12
		}
		result += fmt.Sprintf("### Definitions (%d)\n", len(e.Definitions))
		for i := 0; i < limit; i++ {
			loc := e.Definitions[i]
			result += fmt.Sprintf("- %s:%d:%d\n", loc.Path, loc.Line, loc.Col)
		}
		result += "\n"
	}

	if len(e.References) > 0 {
		limit := len(e.References)
		if limit > 12 {
			limit = 12
		}
		result += fmt.Sprintf("### References (%d)\n", len(e.References))
		for i := 0; i < limit; i++ {
			loc := e.References[i]
			result += fmt.Sprintf("- %s:%d:%d\n", loc.Path, loc.Line, loc.Col)
		}
	}

	return result
}

func severityString(sev int) string {
	switch sev {
	case 1:
		return "error"
	case 2:
		return "warning"
	case 3:
		return "information"
	case 4:
		return "hint"
	default:
		return "unknown"
	}
}

// NormalizeExtension ensures an extension has a leading dot and is lowercase.
func NormalizeExtension(ext string) string {
	if ext != "" && ext[0] != '.' {
		ext = "." + ext
	}
	return strings.ToLower(ext)
}
