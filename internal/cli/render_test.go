package cli

import (
	"regexp"
	"strings"
	"testing"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

func TestRenderMarkdownHeadings(t *testing.T) {
	result := RenderMarkdown("# Hello")
	if !strings.Contains(result, "Hello") {
		t.Errorf("heading not rendered: %q", result)
	}

	result = RenderMarkdown("## Sub Heading")
	if !strings.Contains(result, "Sub Heading") {
		t.Errorf("sub heading not rendered: %q", result)
	}
}

func TestRenderMarkdownBold(t *testing.T) {
	result := RenderMarkdown("This is **bold** text")
	if !strings.Contains(result, "bold") {
		t.Errorf("bold not rendered: %q", result)
	}
}

func TestRenderMarkdownCode(t *testing.T) {
	result := RenderMarkdown("Use `git status` to check")
	if !strings.Contains(result, "git status") {
		t.Errorf("code not rendered: %q", result)
	}
}

func TestRenderMarkdownLinks(t *testing.T) {
	result := RenderMarkdown("[Click here](https://example.com)")
	if !strings.Contains(result, "Click here") {
		t.Errorf("link text not rendered: %q", result)
	}
	if !strings.Contains(result, "https://example.com") {
		t.Errorf("link URL not rendered: %q", result)
	}
}

func TestRenderCodeBlock(t *testing.T) {
	result := RenderCodeBlock("fmt.Println(\"hello\")", "go")
	if !strings.Contains(result, "go") {
		t.Errorf("language not shown: %q", result)
	}
	// Strip ANSI codes to check content
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "fmt.Println") {
		t.Errorf("code not shown: %q", stripped)
	}
}

func TestRenderCodeBlockNoLanguage(t *testing.T) {
	result := RenderCodeBlock("hello", "")
	if !strings.Contains(result, "hello") {
		t.Errorf("code not shown: %q", result)
	}
}

func TestRenderToolCall(t *testing.T) {
	result := RenderToolCall("bash", `{"command":"ls -la"}`)
	if !strings.Contains(result, "bash") {
		t.Errorf("tool name not shown: %q", result)
	}
}

func TestRenderToolCallTruncation(t *testing.T) {
	longInput := strings.Repeat("x", 300)
	result := RenderToolCall("bash", longInput)
	if !strings.Contains(result, "...") {
		t.Errorf("long input should be truncated: %q", result)
	}
}

func TestRenderToolResult(t *testing.T) {
	result := RenderToolResult("success", false)
	if !strings.Contains(result, "success") {
		t.Errorf("result not shown: %q", result)
	}
}

func TestRenderToolResultError(t *testing.T) {
	result := RenderToolResult("command failed", true)
	if !strings.Contains(result, "Error") {
		t.Errorf("error not indicated: %q", result)
	}
}

func TestRenderUsage(t *testing.T) {
	result := RenderUsage(100, 50, 0.0025)
	if !strings.Contains(result, "100") {
		t.Errorf("input tokens not shown: %q", result)
	}
	if !strings.Contains(result, "50") {
		t.Errorf("output tokens not shown: %q", result)
	}
	if !strings.Contains(result, "0.0025") {
		t.Errorf("cost not shown: %q", result)
	}
}

func TestProgressBar(t *testing.T) {
	result := ProgressBar("Processing", 5, 10)
	if !strings.Contains(result, "Processing") {
		t.Errorf("label not shown: %q", result)
	}
	if !strings.Contains(result, "5/10") {
		t.Errorf("count not shown: %q", result)
	}
	if !strings.Contains(result, "50%") {
		t.Errorf("percentage not shown: %q", result)
	}
}
