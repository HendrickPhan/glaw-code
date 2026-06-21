// Package agent re-exports types from internal/modules/agent.
// The canonical implementation lives in the modules structure.
// Existing code importing "internal/agent" continues to work unchanged.
package agent

import (
	agententity "github.com/hieu-glaw/glaw-code/internal/modules/agent/domain/entity"
	agentusecase "github.com/hieu-glaw/glaw-code/internal/modules/agent/application/usecase"
)

// Re-export domain entity types
type AgentStatus = agententity.AgentStatus
type AgentResult = agententity.AgentResult
type AgentJob = agententity.AgentJob
type AgentType = agententity.AgentType
type Agent = agententity.Agent
type SubAgentConfig = agententity.SubAgentConfig

// Re-export entity constants
const (
	StatusPending   = agententity.StatusPending
	StatusRunning   = agententity.StatusRunning
	StatusCompleted = agententity.StatusCompleted
	StatusFailed    = agententity.StatusFailed
	StatusCancelled = agententity.StatusCancelled
)

// Re-export entity variables
var BuiltinSubAgents = agententity.BuiltinSubAgents

// Re-export entity functions
var IsValidAgentType = agententity.IsValidAgentType
var ParseSubAgentConfig = agententity.ParseSubAgentConfig
var LoadSubAgentsFromDir = agententity.LoadSubAgentsFromDir
var LoadAllSubAgents = agententity.LoadAllSubAgents
var CreateSubAgentFile = agententity.CreateSubAgentFile
var EnsureAgentsDir = agententity.EnsureAgentsDir
var EnsureUserAgentsDir = agententity.EnsureUserAgentsDir
var GetBuiltinSubAgent = agententity.GetBuiltinSubAgent
var BuiltinSubAgentNames = agententity.BuiltinSubAgentNames

// Re-export application layer types
type SubAgentExecutor = agentusecase.SubAgentExecutor
type SubAgentOrchestrator = agentusecase.SubAgentOrchestrator
type Manager = agentusecase.Manager
type AgentsProviderAdapter = agentusecase.AgentsProviderAdapter

// Re-export application layer functions
var NewManager = agentusecase.NewManager
var NewAgentsProviderAdapter = agentusecase.NewAgentsProviderAdapter
var NewSubAgentExecutor = agentusecase.NewSubAgentExecutor
var NewSubAgentExecutorWithClient = agentusecase.NewSubAgentExecutorWithClient
var NewSubAgentOrchestrator = agentusecase.NewSubAgentOrchestrator
var NewSubAgentOrchestratorWithClient = agentusecase.NewSubAgentOrchestratorWithClient
var SetCustomConfigs = agentusecase.SetCustomConfigs
var GetCustomConfigs = agentusecase.GetCustomConfigs
var AllAvailableAgents = agentusecase.AllAvailableAgents
var AgentToolSpecs = agentusecase.AgentToolSpecs
