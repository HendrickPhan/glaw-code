package tasks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateAndList(t *testing.T) {
	s := NewStore("")
	t1 := s.Create("Write tests")
	t2 := s.Create("Fix bugs")

	if t1.ID != "1" || t2.ID != "2" {
		t.Fatalf("unexpected IDs: %s, %s", t1.ID, t2.ID)
	}

	list := s.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(list))
	}
}

func TestUpdate(t *testing.T) {
	s := NewStore("")
	s.Create("Do thing")

	updated, err := s.Update("1", StatusInProgress, "doing it now")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != StatusInProgress {
		t.Fatalf("expected in_progress, got %s", updated.Status)
	}
	if updated.Description != "doing it now" {
		t.Fatalf("expected description 'doing it now', got %q", updated.Description)
	}
}

func TestDelete(t *testing.T) {
	s := NewStore("")
	s.Create("Remove me")

	if err := s.Delete("1"); err != nil {
		t.Fatal(err)
	}

	list := s.List()
	if len(list) != 0 {
		t.Fatalf("expected 0 tasks after delete, got %d", len(list))
	}

	_, ok := s.Get("1")
	if ok {
		t.Fatal("expected task to be gone after delete")
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")

	s1 := NewStore(path)
	s1.Create("Persist me")
	s1.Create("And me too")

	// Load from file
	s2 := NewStore(path)
	list := s2.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 persisted tasks, got %d", len(list))
	}

	// Create more in s2, should get ID 3
	t3 := s2.Create("Third task")
	if t3.ID != "3" {
		t.Fatalf("expected ID 3 after reload, got %s", t3.ID)
	}
}

func TestDependencies(t *testing.T) {
	s := NewStore("")
	s.Create("First")
	s.Create("Second")

	err := s.SetDependencies("2", nil, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}

	t2, _ := s.Get("2")
	if len(t2.BlockedBy) != 1 || t2.BlockedBy[0] != "1" {
		t.Fatalf("expected blocked_by [1], got %v", t2.BlockedBy)
	}
}

func TestInvalidStatus(t *testing.T) {
	s := NewStore("")
	s.Create("Test")

	_, err := s.Update("1", "invalid_status", "")
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestGetNonexistent(t *testing.T) {
	s := NewStore("")
	_, ok := s.Get("999")
	if ok {
		t.Fatal("expected false for nonexistent task")
	}
}

func TestDeleteNonexistent(t *testing.T) {
	s := NewStore("")
	err := s.Delete("999")
	if err == nil {
		t.Fatal("expected error for deleting nonexistent task")
	}
}

func TestFileCreated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "tasks.json")

	s := NewStore(path)
	s.Create("Nested dir test")

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("tasks file not created: %v", err)
	}
}
