package workflow

import (
	"context"
	"sync"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
)

// Workflow is the central orchestrator for the application.
//
// Thread Safety: Workflow is NOT safe for concurrent use.
// Only one goroutine should call Run() at a time.
//
// Events Channel Contract: The caller must continuously drain
// the events channel. If the channel fills up, Run() will block.
// Pass nil if events are not needed.
type Workflow struct {
	provider       llmProvider
	toolExecutor   *toolExecutor
	sessionStore   sessionStore
	currentSession *domain.Session
	currentModel   string
	events         chan<- Event
	cfg            *config.Config

	// Run lifecycle management
	runCancel context.CancelFunc
	runDone   chan struct{}
	mu        sync.Mutex
}

// NewWorkflow creates a new Workflow with all dependencies.
func NewWorkflow(
	provider llmProvider,
	toolRegistry toolRegistry,
	sessionStore sessionStore,
	cfg *config.Config,
	events chan<- Event,
) *Workflow {
	if cfg == nil {
		panic("cfg is required")
	}
	return &Workflow{
		provider:     provider,
		toolExecutor: newToolExecutor(toolRegistry),
		sessionStore: sessionStore,
		events:       events,
		cfg:          cfg,
	}
}
