package workflow

import (
	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
)

// Workflow is the central orchestrator for the application.
type Workflow struct {
	llmRegistry    llmRegistry
	currentLLM     domain.LLM
	toolExecutor   *toolExecutor
	sessionStore   sessionStore
	currentSession *domain.Session
	events         chan<- domain.Event
	cfg            *config.Config
}

// NewWorkflow creates a new Workflow with all dependencies.
func NewWorkflow(
	llmRegistry llmRegistry,
	toolRegistry toolRegistry,
	sessionStore sessionStore,
	cfg *config.Config,
	events chan<- domain.Event,
) *Workflow {
	if cfg == nil {
		panic("cfg is required")
	}
	return &Workflow{
		llmRegistry:  llmRegistry,
		toolExecutor: newToolExecutor(toolRegistry),
		sessionStore: sessionStore,
		events:       events,
		cfg:          cfg,
	}
}
