package lsp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDiagnosticSeverity(t *testing.T) {
	d := Diagnostic{
		Severity: 1,
		Message:  "undefined variable",
		Range: Range{
			Start: Position{Line: 5, Character: 10},
			End:   Position{Line: 5, Character: 15},
		},
	}
	if d.Severity != 1 {
		t.Errorf("Severity = %d, want 1", d.Severity)
	}
	if d.Message != "undefined variable" {
		t.Errorf("Message = %q", d.Message)
	}
}

func TestRangeAndPosition(t *testing.T) {
	r := Range{Start: Position{Line: 1, Character: 0}, End: Position{Line: 1, Character: 5}}
	if r.Start.Line != 1 {
		t.Errorf("Start.Line = %d", r.Start.Line)
	}
	if r.End.Character != 5 {
		t.Errorf("End.Character = %d", r.End.Character)
	}
}

func TestSymbolLocation(t *testing.T) {
	sl := SymbolLocation{
		Path: "main.go",
		Line: 10,
		Col:  5,
	}
	if sl.Path != "main.go" {
		t.Errorf("Path = %q", sl.Path)
	}
	if sl.Line != 10 {
		t.Errorf("Line = %d", sl.Line)
	}
}

func TestFileDiagnostics(t *testing.T) {
	fd := FileDiagnostics{
		Path: "test.go",
		URI:  "file:///test.go",
		Diagnostics: []Diagnostic{
			{Severity: 1, Message: "err"},
			{Severity: 2, Message: "warn"},
		},
	}
	if len(fd.Diagnostics) != 2 {
		t.Errorf("len = %d, want 2", len(fd.Diagnostics))
	}
}

func TestWorkspaceDiagnostics(t *testing.T) {
	wd := WorkspaceDiagnostics{
		Files: []FileDiagnostics{
			{Path: "a.go", Diagnostics: []Diagnostic{{Message: "x"}}},
			{Path: "b.go", Diagnostics: []Diagnostic{{Message: "y"}}},
		},
	}
	if len(wd.Files) != 2 {
		t.Errorf("len = %d, want 2", len(wd.Files))
	}
}

func TestWorkspaceDiagnosticsIsEmpty(t *testing.T) {
	wd := WorkspaceDiagnostics{}
	if !wd.IsEmpty() {
		t.Error("empty workspace should be IsEmpty")
	}
	wd.Files = append(wd.Files, FileDiagnostics{Path: "a.go", Diagnostics: []Diagnostic{{Message: "x"}}})
	if wd.IsEmpty() {
		t.Error("workspace with files should not be IsEmpty")
	}
}

func TestWorkspaceDiagnosticsTotal(t *testing.T) {
	wd := WorkspaceDiagnostics{
		Files: []FileDiagnostics{
			{Path: "a.go", Diagnostics: []Diagnostic{{Message: "x"}, {Message: "y"}}},
			{Path: "b.go", Diagnostics: []Diagnostic{{Message: "z"}}},
		},
	}
	if wd.TotalDiagnostics() != 3 {
		t.Errorf("TotalDiagnostics = %d, want 3", wd.TotalDiagnostics())
	}
}

func TestServerConfig(t *testing.T) {
	sc := ServerConfig{
		Name:    "gopls",
		Command: "gopls",
		Args:    []string{"serve"},
	}
	data, err := json.Marshal(sc)
	if err != nil {
		t.Fatal(err)
	}
	var parsed ServerConfig
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Command != "gopls" {
		t.Errorf("Command = %q", parsed.Command)
	}
}

func TestContextEnrichment(t *testing.T) {
	ce := ContextEnrichment{
		FilePath: "main.go",
		Diagnostics: []Diagnostic{
			{Severity: 1, Message: "undefined: x", Range: Range{Start: Position{Line: 5, Character: 0}, End: Position{Line: 5, Character: 1}}},
		},
		Definitions: []SymbolLocation{
			{Path: "main.go", Line: 10, Col: 5},
		},
	}
	rendered := ce.RenderPromptSection()
	if len(rendered) == 0 {
		t.Error("RenderPromptSection should not be empty")
	}
	if !strings.Contains(rendered, "LSP Context") {
		t.Error("should contain 'LSP Context'")
	}
}

func TestContextEnrichmentIsEmpty(t *testing.T) {
	ce := ContextEnrichment{}
	if !ce.IsEmpty() {
		t.Error("empty enrichment should be IsEmpty")
	}
	ce.Diagnostics = []Diagnostic{{Message: "x"}}
	if ce.IsEmpty() {
		t.Error("enrichment with diagnostics should not be IsEmpty")
	}
}

func TestNormalizeExtension(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"go", ".go"},
		{".go", ".go"},
		{"ts", ".ts"},
		{".ts", ".ts"},
		{"", ""},
		{"Go", ".go"},
	}
	for _, tt := range tests {
		got := NormalizeExtension(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeExtension(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSeverityString(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{1, "error"},
		{2, "warning"},
		{3, "information"},
		{4, "hint"},
		{99, "unknown"},
	}
	for _, tt := range tests {
		got := severityString(tt.input)
		if got != tt.want {
			t.Errorf("severityString(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNewClient(t *testing.T) {
	c := NewClient(ServerConfig{Command: "gopls", Args: []string{"serve"}})
	if c == nil {
		t.Fatal("client should not be nil")
	}
}

func TestNewManager(t *testing.T) {
	m, err := NewManager(nil)
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}
	if m == nil {
		t.Fatal("manager should not be nil")
	}
}

func TestNewManagerWithConfig(t *testing.T) {
	m, err := NewManager([]ServerConfig{
		{
			Name:                "gopls",
			Command:             "gopls",
			Args:                []string{"serve"},
			ExtensionToLanguage: map[string]string{".go": "go"},
		},
	})
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}
	if !m.SupportsPath("main.go") {
		t.Error("should support .go files")
	}
	if m.SupportsPath("main.py") {
		t.Error("should not support .py files")
	}
}

func TestManagerShutdown(t *testing.T) {
	m, _ := NewManager(nil)
	if err := m.Shutdown(); err != nil {
		t.Errorf("Shutdown error: %v", err)
	}
}

func TestDedupeLocations(t *testing.T) {
	locs := []SymbolLocation{
		{Path: "a.go", Line: 1, Col: 1},
		{Path: "a.go", Line: 1, Col: 1},
		{Path: "b.go", Line: 2, Col: 2},
	}
	deduped := DedupeLocations(locs)
	if len(deduped) != 2 {
		t.Errorf("DedupeLocations = %d, want 2", len(deduped))
	}
}
