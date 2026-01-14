package workflow

import (
	"context"
	"fmt"
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
	providerRegistry providerRegistry
	currentProvider  domain.Provider
	toolExecutor     *toolExecutor
	sessionStore     sessionStore
	currentSession   *domain.Session
	currentModel     string
	events           chan<- Event
	cfg              *config.Config

	// Run lifecycle management
	runCancel context.CancelFunc
	runDone   chan struct{}
	mu        sync.Mutex
}

// NewWorkflow creates a new Workflow with all dependencies.
func NewWorkflow(
	providerRegistry providerRegistry,
	toolRegistry toolRegistry,
	sessionStore sessionStore,
	cfg *config.Config,
	events chan<- Event,
) *Workflow {
	if cfg == nil {
		panic("cfg is required")
	}
	return &Workflow{
		providerRegistry: providerRegistry,
		toolExecutor:     newToolExecutor(toolRegistry),
		sessionStore:     sessionStore,
		events:           events,
		cfg:              cfg,
	}
}

// SetProvider sets the current provider by name.
func (w *Workflow) SetProvider(name string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	p, ok := w.providerRegistry.Get(name)
	if !ok {
		return fmt.Errorf("unknown provider: %s", name)
	}
	w.currentProvider = p
	return nil
}

// ListProviders returns all registered provider names.
func (w *Workflow) ListProviders() []string {
	return w.providerRegistry.List()
}
