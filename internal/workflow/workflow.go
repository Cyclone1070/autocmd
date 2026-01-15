package workflow

import (
	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
)

// Workflow is the central orchestrator for the application.
type Workflow struct {
	modelRegistry  modelRegistry
	currentModel   domain.Model
	toolExecutor   *toolExecutor
	sessionStore   sessionStore
	currentSession *domain.Session
	events         chan<- Event
	cfg            *config.Config
}

// NewWorkflow creates a new Workflow with all dependencies.
func NewWorkflow(
	modelRegistry modelRegistry,
	toolRegistry toolRegistry,
	sessionStore sessionStore,
	cfg *config.Config,
	events chan<- Event,
) *Workflow {
	if cfg == nil {
		panic("cfg is required")
	}
	return &Workflow{
		modelRegistry: modelRegistry,
		toolExecutor:  newToolExecutor(toolRegistry),
		sessionStore:  sessionStore,
		events:        events,
		cfg:           cfg,
	}
}
