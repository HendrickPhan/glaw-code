package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotWriteFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.txt")

	// Create initial file
	if err := os.WriteFile(file, []byte("original"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	inner := NewBuiltinToolExecutor(dir)
	exec := NewSnapshottingExecutor(inner)

	exec.BeginBatch()

	// Write new content (should snapshot "original")
	input, err := json.Marshal(map[string]string{
		"path":    file,
		"content": "modified",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	_, err = exec.ExecuteTool(context.Background(), "write_file", input)
	if err != nil {
		t.Fatal(err)
	}

	// Verify file was modified
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "modified" {
		t.Fatalf("expected 'modified', got %q", string(data))
	}

	exec.FinishBatch()

	// Revert
	count, err := exec.RevertLastTurn()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 file reverted, got %d", count)
	}

	// Verify original content restored
	data, err = os.ReadFile(file)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "original" {
		t.Fatalf("expected 'original' after revert, got %q", string(data))
	}
}

func TestSnapshotNewFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "newfile.txt")

	// File does NOT exist yet
	inner := NewBuiltinToolExecutor(dir)
	exec := NewSnapshottingExecutor(inner)

	exec.BeginBatch()

	input, err := json.Marshal(map[string]string{
		"path":    file,
		"content": "brand new",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	_, err = exec.ExecuteTool(context.Background(), "write_file", input)
	if err != nil {
		t.Fatal(err)
	}

	// Verify file was created
	if _, err := os.Stat(file); os.IsNotExist(err) {
		t.Fatal("file should exist")
	}

	exec.FinishBatch()

	// Revert should remove the file
	count, err := exec.RevertLastTurn()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}

	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("file should be removed after revert")
	}
}

func TestSnapshotEditFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "edit.txt")
	if err := os.WriteFile(file, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	inner := NewBuiltinToolExecutor(dir)
	exec := NewSnapshottingExecutor(inner)

	exec.BeginBatch()

	input, err := json.Marshal(map[string]string{
		"path":       file,
		"old_string": "hello",
		"new_string": "goodbye",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	_, err = exec.ExecuteTool(context.Background(), "edit_file", input)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "goodbye world" {
		t.Fatalf("expected 'goodbye world', got %q", string(data))
	}

	exec.FinishBatch()

	count, err := exec.RevertLastTurn()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}

	data, err = os.ReadFile(file)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello world" {
		t.Fatalf("expected 'hello world' after revert, got %q", string(data))
	}
}

func TestRevertAll(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "a.txt")
	file2 := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(file1, []byte("one"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(file2, []byte("two"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	inner := NewBuiltinToolExecutor(dir)
	exec := NewSnapshottingExecutor(inner)

	// Batch 1: modify file1
	exec.BeginBatch()
	input, err := json.Marshal(map[string]string{"path": file1, "content": "ONE"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if _, err := exec.ExecuteTool(context.Background(), "write_file", input); err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	exec.FinishBatch()

	// Batch 2: modify file2
	exec.BeginBatch()
	input, err = json.Marshal(map[string]string{"path": file2, "content": "TWO"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if _, err := exec.ExecuteTool(context.Background(), "write_file", input); err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	exec.FinishBatch()

	// Revert all
	count, err := exec.RevertAll()
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2, got %d", count)
	}

	data, err := os.ReadFile(file1)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "one" {
		t.Fatalf("expected 'one', got %q", string(data))
	}
	data, err = os.ReadFile(file2)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "two" {
		t.Fatalf("expected 'two', got %q", string(data))
	}
}

func TestRevertEmpty(t *testing.T) {
	dir := t.TempDir()
	inner := NewBuiltinToolExecutor(dir)
	exec := NewSnapshottingExecutor(inner)

	_, err := exec.RevertLastTurn()
	if err == nil {
		t.Fatal("expected error when no changes to revert")
	}
}

func TestDontSnapshotSameFileTwice(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "dup.txt")
	if err := os.WriteFile(file, []byte("v1"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	inner := NewBuiltinToolExecutor(dir)
	exec := NewSnapshottingExecutor(inner)

	exec.BeginBatch()

	// Write twice to same file in one batch
	input, err := json.Marshal(map[string]string{"path": file, "content": "v2"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if _, err := exec.ExecuteTool(context.Background(), "write_file", input); err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	input, err = json.Marshal(map[string]string{"path": file, "content": "v3"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if _, err := exec.ExecuteTool(context.Background(), "write_file", input); err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}

	exec.FinishBatch()

	// Should only revert to the FIRST snapshot (v1)
	count, err := exec.RevertLastTurn()
	if err != nil {
		t.Fatalf("RevertLastTurn: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "v1" {
		t.Fatalf("expected 'v1' after revert, got %q", string(data))
	}
}
