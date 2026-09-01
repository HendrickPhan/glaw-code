package entity

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SystemPromptBuilder constructs system prompts.
type SystemPromptBuilder struct {
	ProjectContext     string
	InstructionFiles   map[string]string
	GitStatus          string
	GitDiff            string
	ToolDescriptions   []string
	CustomInstructions string
}

// NewSystemPromptBuilder creates a new builder.
func NewSystemPromptBuilder() *SystemPromptBuilder {
	return &SystemPromptBuilder{
		InstructionFiles: make(map[string]string),
	}
}

// Build assembles the system prompt.
func (b *SystemPromptBuilder) Build() string {
	var parts []string

	parts = append(parts, `You are glaw-code, an AI coding assistant. You help users with software engineering tasks.

IMPORTANT: You have access to tools. When the user asks you to create files, edit code, run commands, or perform any action, you MUST use the appropriate tool rather than just describing what you would do. For example:
- To create or write files, use the write_file tool
- To edit existing files, use the edit_file tool
- To read files, use the read_file tool
- To run commands, use the bash tool
- To search for files, use glob_search
- To search file contents, use grep_search

Always execute the actual tool calls to accomplish the task. Do NOT just describe what you would do - actually do it.`)

	if b.ProjectContext != "" {
		parts = append(parts, "\n## Project Context\n"+b.ProjectContext)
	}

	for name, content := range b.InstructionFiles {
		parts = append(parts, fmt.Sprintf("\n## Instructions from %s\n%s", name, content))
	}

	if b.GitStatus != "" {
		parts = append(parts, "\n## Git Status\n"+b.GitStatus)
	}

	if b.ToolDescriptions != nil {
		parts = append(parts, "\n## Available Tools")
		parts = append(parts, b.ToolDescriptions...)
	}

	if b.CustomInstructions != "" {
		parts = append(parts, "\n## Custom Instructions\n"+b.CustomInstructions)
	}

	return join(parts, "\n")
}

func join(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

// LoadInstructionFiles discovers and loads GLAW.md (and CLAW.md backward-compat)
// instruction files from the workspace root.
func LoadInstructionFiles(root string) map[string]string {
	files := make(map[string]string)

	loadInstructionFile(files, root, "GLAW.md")
	loadInstructionFile(files, root, ".glaw/GLAW.md")
	loadInstructionDir(files, root, ".glaw/instructions")
	loadInstructionFile(files, root, "CLAW.md")
	loadInstructionFile(files, root, ".glaw/CLAW.md")

	return files
}

func loadInstructionFile(files map[string]string, root, relPath string) {
	fullPath := filepath.Join(root, relPath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return
	}
	files[relPath] = string(data)
}

func loadInstructionDir(files map[string]string, root, relDir string) {
	dirPath := filepath.Join(root, relDir)
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		relPath := filepath.Join(relDir, entry.Name())
		loadInstructionFile(files, root, relPath)
	}
}
