// Package tasks re-exports types from internal/modules/tasks/domain/entity.
// The canonical implementation lives in the modules structure.
package tasks

import taskentity "github.com/hieu-glaw/glaw-code/internal/modules/tasks/domain/entity"

// Re-export all types
type Task = taskentity.Task
type Store = taskentity.Store

// Re-export constants
const StatusPending = taskentity.StatusPending
const StatusInProgress = taskentity.StatusInProgress
const StatusCompleted = taskentity.StatusCompleted
const StatusDeleted = taskentity.StatusDeleted

// Re-export functions
var NewStore = taskentity.NewStore
