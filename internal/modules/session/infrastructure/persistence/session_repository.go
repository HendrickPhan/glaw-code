package persistence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hieu-glaw/glaw-code/internal/modules/session/domain/entity"
)

// SaveSession persists a session to disk.
func SaveSession(session *entity.Session, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating session dir: %w", err)
	}

	path := filepath.Join(dir, session.ID+".json")
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling session: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("writing session file: %w", err)
	}

	return path, nil
}

// LoadSession reads a session from disk.
func LoadSession(path string) (*entity.Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading session file: %w", err)
	}

	var session entity.Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("parsing session file: %w", err)
	}

	return &session, nil
}
