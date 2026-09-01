package persistence

import (
	"testing"

	"github.com/hieu-glaw/glaw-code/internal/shared/api"
	sessionentity "github.com/hieu-glaw/glaw-code/internal/modules/session/domain/entity"
)

func TestSaveAndLoadSession(t *testing.T) {
	dir := t.TempDir()
	s := sessionentity.NewSession()
	s.AddUserMessageFromText("hello")
	s.AddAssistantMessage([]api.ContentBlock{api.NewTextBlock("world")}, &api.Usage{InputTokens: 5, OutputTokens: 10})

	path, err := SaveSession(s, dir)
	if err != nil {
		t.Fatalf("SaveSession error: %v", err)
	}

	loaded, err := LoadSession(path)
	if err != nil {
		t.Fatalf("LoadSession error: %v", err)
	}
	if loaded.ID != s.ID {
		t.Errorf("ID = %q, want %q", loaded.ID, s.ID)
	}
	if loaded.Version != s.Version {
		t.Errorf("Version = %d, want %d", loaded.Version, s.Version)
	}
	if loaded.MessageCount() != 2 {
		t.Errorf("MessageCount = %d, want 2", loaded.MessageCount())
	}
}

func TestLoadSessionNotFound(t *testing.T) {
	_, err := LoadSession("nonexistent.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}
