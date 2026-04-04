package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/hieu-glaw/glaw-code/internal/api"
)

// FileSnapshot captures a file's state before a modification.
// If Existed is false, the file did not exist before the operation.
type FileSnapshot struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Existed bool   `json:"existed"`
}

// SnapshotBatch groups snapshots from a single user interaction.
type SnapshotBatch struct {
	Snapshots []FileSnapshot
}

// SnapshottingExecutor wraps a ToolExecutor and captures file state
// before write_file and edit_file operations, enabling undo.
type SnapshottingExecutor struct {
	inner   ToolExecutor
	mu      sync.Mutex
	batches []SnapshotBatch
	active  *SnapshotBatch
}

// NewSnapshottingExecutor creates a new snapshotting wrapper around an executor.
func NewSnapshottingExecutor(inner ToolExecutor) *SnapshottingExecutor {
	return &SnapshottingExecutor{
		inner: inner,
	}
}

// BeginBatch starts capturing snapshots for a new user interaction cycle.
func (e *SnapshottingExecutor) BeginBatch() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.active = &SnapshotBatch{}
}

// FinishBatch finalizes the current batch and adds it to the history.
func (e *SnapshottingExecutor) FinishBatch() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.active != nil && len(e.active.Snapshots) > 0 {
		e.batches = append(e.batches, *e.active)
	}
	e.active = nil
}

// ExecuteTool implements ToolExecutor. It snapshots file state before
// write_file and edit_file operations, then delegates to the inner executor.
func (e *SnapshottingExecutor) ExecuteTool(ctx context.Context, name string, input json.RawMessage) (*ToolOutput, error) {
	switch name {
	case "write_file":
		e.snapshotFile(input)
	case "edit_file":
		e.snapshotFile(input)
	}
	return e.inner.ExecuteTool(ctx, name, input)
}

// GetToolSpecs passes through to the inner executor.
func (e *SnapshottingExecutor) GetToolSpecs() []api.ToolDefinition {
	return e.inner.GetToolSpecs()
}

// snapshotFile reads a file's current content and stores it as a snapshot.
func (e *SnapshottingExecutor) snapshotFile(input json.RawMessage) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &args); err != nil || args.Path == "" {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.active == nil {
		// No batch active; start one implicitly
		e.active = &SnapshotBatch{}
	}

	// Don't snapshot the same file twice in one batch
	for _, s := range e.active.Snapshots {
		if s.Path == args.Path {
			return
		}
	}

	data, err := os.ReadFile(args.Path)
	if err != nil {
		// File doesn't exist yet — record that
		e.active.Snapshots = append(e.active.Snapshots, FileSnapshot{
			Path:    args.Path,
			Existed: false,
		})
		return
	}

	e.active.Snapshots = append(e.active.Snapshots, FileSnapshot{
		Path:    args.Path,
		Content: string(data),
		Existed: true,
	})
}

// RevertLastTurn restores all files modified in the most recent batch.
func (e *SnapshottingExecutor) RevertLastTurn() (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// If there's an active batch with snapshots, finalize it first
	if e.active != nil && len(e.active.Snapshots) > 0 {
		e.batches = append(e.batches, *e.active)
		e.active = nil
	}

	if len(e.batches) == 0 {
		return 0, fmt.Errorf("no changes to revert")
	}

	last := e.batches[len(e.batches)-1]
	count, err := e.restoreSnapshots(last.Snapshots)
	if err != nil {
		return count, err
	}

	e.batches = e.batches[:len(e.batches)-1]
	return count, nil
}

// RevertAll restores all files modified across all batches.
func (e *SnapshottingExecutor) RevertAll() (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Include active batch
	if e.active != nil && len(e.active.Snapshots) > 0 {
		e.batches = append(e.batches, *e.active)
		e.active = nil
	}

	if len(e.batches) == 0 {
		return 0, fmt.Errorf("no changes to revert")
	}

	total := 0
	for i := len(e.batches) - 1; i >= 0; i-- {
		count, err := e.restoreSnapshots(e.batches[i].Snapshots)
		total += count
		if err != nil {
			return total, err
		}
	}

	e.batches = nil
	return total, nil
}

// restoreSnapshots writes back the original file contents.
func (e *SnapshottingExecutor) restoreSnapshots(snapshots []FileSnapshot) (int, error) {
	count := 0
	for _, snap := range snapshots {
		if snap.Existed {
			if err := os.WriteFile(snap.Path, []byte(snap.Content), 0o644); err != nil {
				return count, fmt.Errorf("restoring %s: %w", snap.Path, err)
			}
		} else {
			if err := os.Remove(snap.Path); err != nil && !os.IsNotExist(err) {
				return count, fmt.Errorf("removing %s: %w", snap.Path, err)
			}
		}
		count++
	}
	return count, nil
}
