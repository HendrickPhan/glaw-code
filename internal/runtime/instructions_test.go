package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadInstructionFiles(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()

	// No files yet - should return empty
	files := LoadInstructionFiles(tmpDir)
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}

	// Create GLAW.md at root
	glawContent := "# Project Rules\nAlways use Go conventions."
	if err := os.WriteFile(filepath.Join(tmpDir, "GLAW.md"), []byte(glawContent), 0o644); err != nil {
		t.Fatal(err)
	}

	files = LoadInstructionFiles(tmpDir)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d: %v", len(files), files)
	}
	if files["GLAW.md"] != glawContent {
		t.Errorf("GLAW.md content mismatch: got %q", files["GLAW.md"])
	}

	// Create .glaw/GLAW.md
	os.MkdirAll(filepath.Join(tmpDir, ".glaw"), 0o755)
	glawLocal := "# Local Rules\nUse tabs for indentation."
	if err := os.WriteFile(filepath.Join(tmpDir, ".glaw", "GLAW.md"), []byte(glawLocal), 0o644); err != nil {
		t.Fatal(err)
	}

	files = LoadInstructionFiles(tmpDir)
	if len(files) < 2 {
		t.Fatalf("expected >= 2 files, got %d: %v", len(files), files)
	}
	if files[".glaw/GLAW.md"] != glawLocal {
		t.Errorf(".glaw/GLAW.md content mismatch")
	}

	// Create .glaw/instructions/test-rules.md
	os.MkdirAll(filepath.Join(tmpDir, ".glaw", "instructions"), 0o755)
	instrContent := "# Test Rules\nRun tests before committing."
	if err := os.WriteFile(filepath.Join(tmpDir, ".glaw", "instructions", "test-rules.md"), []byte(instrContent), 0o644); err != nil {
		t.Fatal(err)
	}

	files = LoadInstructionFiles(tmpDir)
	if len(files) < 3 {
		t.Fatalf("expected >= 3 files, got %d: %v", len(files), files)
	}
	instrKey := filepath.Join(".glaw", "instructions", "test-rules.md")
	if files[instrKey] != instrContent {
		t.Errorf("instructions/test-rules.md content mismatch")
	}

	// Backward compat: CLAW.md
	clawContent := "# Legacy Rules\nOld conventions."
	if err := os.WriteFile(filepath.Join(tmpDir, "CLAW.md"), []byte(clawContent), 0o644); err != nil {
		t.Fatal(err)
	}

	files = LoadInstructionFiles(tmpDir)
	if files["CLAW.md"] != clawContent {
		t.Errorf("CLAW.md backward compat failed")
	}
}

func TestLoadInstructionFilesNonexistent(t *testing.T) {
	files := LoadInstructionFiles("/nonexistent/path")
	if len(files) != 0 {
		t.Errorf("expected 0 files for nonexistent path, got %d", len(files))
	}
}
