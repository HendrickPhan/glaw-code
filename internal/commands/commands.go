// Package commands re-exports types from internal/modules/commands/domain/entity.
// The canonical implementation lives in the modules structure.
package commands

import cmdentity "github.com/hieu-glaw/glaw-code/internal/modules/commands/domain/entity"

// Re-export all types
type AgentJobStatus = cmdentity.AgentJobStatus
type Category = cmdentity.Category
type Spec = cmdentity.Spec
type Result = cmdentity.Result
type AgentInfo = cmdentity.AgentInfo
type SubAgentSessionInfo = cmdentity.SubAgentSessionInfo
type AgentsProvider = cmdentity.AgentsProvider
type Runtime = cmdentity.Runtime
type UsageInfo = cmdentity.UsageInfo
type ParsedCommand = cmdentity.ParsedCommand
type Dispatcher = cmdentity.Dispatcher

// Re-export constants
const CategoryCore = cmdentity.CategoryCore
const CategoryWorkspace = cmdentity.CategoryWorkspace
const CategorySession = cmdentity.CategorySession
const CategoryGit = cmdentity.CategoryGit
const CategoryAutomation = cmdentity.CategoryAutomation

// Re-export variables
var Specs = cmdentity.Specs

// Re-export functions
var Parse = cmdentity.Parse
var SuggestCommands = cmdentity.SuggestCommands
var NewDispatcher = cmdentity.NewDispatcher
