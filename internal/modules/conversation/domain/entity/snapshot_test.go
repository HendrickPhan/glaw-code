package entity

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	api "github.com/hieu-glaw/glaw-code/internal/api"
)

// mockToolExecutor is a simple mock for testing.
type mockToolExecutor struct {
	workspaceDir string
}

func (m *mockToolExecutor) ExecuteTool(_ context.Context, name string, input json.RawMessage) (*ToolOutput, error) {
	switch name {
	case "write_file":
		var args struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(input, &args); err != nil {
			return nil, err
		}
		if err := os.WriteFile(args.Path, []byte(args.Content), 0o644); err != nil {
			return &ToolOutput{Content: err.Error(), IsError: true}, nil
		}
		return &ToolOutput{Content: "ok"}, nil
	case "edit_file":
		var args struct {
			Path      string `json:"path"`
			OldString string `json:"old_string"`
			NewString string `json:"new_string"`
		}
		if err := json.Unmarshal(input, &args); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(args.Path)
		if err != nil {
			return &ToolOutput{Content: err.Error(), IsError: true}, nil
		}
		content := string(data)
		newContent := replaceFirst(content, args.OldString, args.NewString)
		if err := os.WriteFile(args.Path, []byte(newContent), 0o644); err != nil {
			return &ToolOutput{Content: err.Error(), IsError: true}, nil
		}
		return &ToolOutput{Content: "ok"}, nil
	default:
		return &ToolOutput{Content: "ok"}, nil
	}
}

func replaceFirst(s, old, new_ string) string {
	return strings.Replace(s, old, new_, 1)
}

func (m *mockToolExecutor) GetToolSpecs() []api.ToolDefinition { return nil }

func TestSnapshottingExecutorWriteFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(filePath, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	exec := NewSnapshottingExecutor(&mockToolExecutor{})
	exec.BeginBatch()

	input := json.RawMessage(`{"path":"` + filePath + `","content":"new"}`)
	_, err := exec.ExecuteTool(context.Background(), "write_file", input)
	if err != nil {
		t.Fatal(err)
	}

	// Verify file was written
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Errorf("content = %q, want %q", string(data), "new")
	}

	// Snapshot should exist
	if len(exec.active.Snapshots) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(exec.active.Snapshots))
	}
	if exec.active.Snapshots[0].Content != "original" {
		t.Errorf("snapshot content = %q", exec.active.Snapshots[0].Content)
	}
}

func TestSnapshottingExecutorRevertLastTurn(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(filePath, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	exec := NewSnapshottingExecutor(&mockToolExecutor{})
	exec.BeginBatch()

	input := json.RawMessage(`{"path":"` + filePath + `","content":"modified"}`)
	_, _ = exec.ExecuteTool(context.Background(), "write_file", input)
	exec.FinishBatch()

	count, err := exec.RevertLastTurn()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	data, _ := os.ReadFile(filePath)
	if string(data) != "original" {
		t.Errorf("after revert: content = %q, want %q", string(data), "original")
	}
}

func TestSnapshottingExecutorRevertAll(t *testing.T) {
	dir := t.TempDir()

	exec := NewSnapshottingExecutor(&mockToolExecutor{})

	// Batch 1
	file1 := filepath.Join(dir, "a.txt")
	os.WriteFile(file1, []byte("orig-a"), 0o644)
	exec.BeginBatch()
	input := json.RawMessage(`{"path":"` + file1 + `","content":"mod-a"}`)
	_, _ = exec.ExecuteTool(context.Background(), "write_file", input)
	exec.FinishBatch()

	// Batch 2
	file2 := filepath.Join(dir, "b.txt")
	os.WriteFile(file2, []byte("orig-b"), 0o644)
	exec.BeginBatch()
	input = json.RawMessage(`{"path":"` + file2 + `","content":"mod-b"}`)
	_, _ = exec.ExecuteTool(context.Background(), "write_file", input)
	exec.FinishBatch()

	count, err := exec.RevertAll()
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}

	data1, _ := os.ReadFile(file1)
	data2, _ := os.ReadFile(file2)
	if string(data1) != "orig-a" {
		t.Errorf("file1 = %q, want %q", string(data1), "orig-a")
	}
	if string(data2) != "orig-b" {
		t.Errorf("file2 = %q, want %q", string(data2), "orig-b")
	}
}

func TestSnapshottingExecutorRevertEmpty(t *testing.T) {
	exec := NewSnapshottingExecutor(&mockToolExecutor{})
	_, err := exec.RevertLastTurn()
	if err == nil {
		t.Error("expected error when no batches to revert")
	}
}

func TestSnapshottingExecutorNewFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "new.txt")

	exec := NewSnapshottingExecutor(&mockToolExecutor{})
	exec.BeginBatch()

	input := json.RawMessage(`{"path":"` + filePath + `","content":"new file"}`)
	_, _ = exec.ExecuteTool(context.Background(), "write_file", input)
	exec.FinishBatch()

	// File should exist now
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("file should exist: %v", err)
	}

	// Revert should remove the file
	count, _ := exec.RevertLastTurn()
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("file should be removed after revert")
	}
}
