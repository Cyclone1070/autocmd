package workflow

import (
	"context"
	"fmt"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
)

// Workflow is the central orchestrator for the application.
type Workflow struct {
	llmRegistry    llmRegistry
	llm            domain.LLM
	toolExecutor   *toolExecutor
	sessionStore   sessionStore
	currentSession *domain.Session
	events         chan<- domain.Event
	cfg            *config.Config
}

// NewWorkflow creates a new Workflow with all dependencies.
func NewWorkflow(
	ctx context.Context,
	llmRegistry llmRegistry,
	toolRegistry toolRegistry,
	sessionStore sessionStore,
	cfg *config.Config,
	events chan<- domain.Event,
) (*Workflow, error) {
	if cfg == nil {
		panic("cfg is required")
	}

	model, err := llmRegistry.Get(ctx, cfg.Model)
	if err != nil {
		return nil, fmt.Errorf("resolve model %q: %w", cfg.Model, err)
	}

	return &Workflow{
		llmRegistry:  llmRegistry,
		llm:          model,
		toolExecutor: newToolExecutor(toolRegistry),
		sessionStore: sessionStore,
		events:       events,
		cfg:          cfg,
	}, nil
}
