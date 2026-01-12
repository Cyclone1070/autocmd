package workflow

import (
	"context"
	"fmt"
	"sync"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/provider"
	"github.com/Cyclone1070/iav/internal/session"
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
	toolManager    *toolManager
	sessionStore   sessionStore
	currentSession *session.Session
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
	sessionStore sessionStore,
	cfg *config.Config,
	events chan<- Event,
	tools []Tool,
) *Workflow {
	if cfg == nil {
		panic("cfg is required")
	}
	return &Workflow{
		provider:     provider,
		toolManager:  newToolManager(tools),
		sessionStore: sessionStore,
		events:       events,
		cfg:          cfg,
	}
}

// Run executes the main LLM-tool interaction loop.
func (w *Workflow) Run(ctx context.Context, input string) error {
	// Create cancellable child context for internal cancellation
	runCtx, runCancel := context.WithCancel(ctx)
	runDone := make(chan struct{})

	w.mu.Lock()
	w.runCancel = runCancel
	w.runDone = runDone
	w.mu.Unlock()

	defer func() {
		close(runDone)
		w.mu.Lock()
		w.runCancel = nil
		w.runDone = nil
		w.mu.Unlock()
	}()

	// Auto-create session if nil
	if w.currentSession == nil {
		s, err := w.sessionStore.Create()
		if err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		w.currentSession = s
	}

	// Capture session and model at start to avoid race with SwitchSession/SetModel
	sess := w.currentSession
	model := w.currentModel

	sess.Add(provider.Message{
		Role:    provider.RoleUser,
		Content: input,
	})

	defer func() {
		if w.events != nil {
			// Non-blocking send to prevent deadlock if channel is full.
			// This defer must complete so runDone can be closed.
			select {
			case w.events <- DoneEvent{}:
			default:
			}
		}
	}()

	maxIterations := w.cfg.Tools.MaxIterations
	for range maxIterations {
		if err := runCtx.Err(); err != nil {
			sess.Add(provider.Message{
				Role:    provider.RoleUser,
				Content: "[Session cancelled by user]",
			})
			_ = w.sessionStore.Save(sess)
			return err
		}

		if w.events != nil {
			w.events <- ThinkingEvent{}
		}

		resp, err := w.provider.Generate(runCtx, model, sess.Messages(), w.toolManager.declarations())
		if err != nil {
			_ = w.sessionStore.Save(sess)
			return fmt.Errorf("provider.Generate: %w", err)
		}
		if resp == nil {
			_ = w.sessionStore.Save(sess)
			return fmt.Errorf("provider.Generate returned nil response")
		}

		sess.Add(*resp)

		if resp.Content != "" && w.events != nil {
			w.events <- TextEvent{Text: resp.Content}
		}

		if len(resp.ToolCalls) == 0 {
			_ = w.sessionStore.Save(sess)
			return nil
		}

		for _, tc := range resp.ToolCalls {
			toolResp, err := w.toolManager.execute(runCtx, tc, w.events)
			if err != nil {
				_ = w.sessionStore.Save(sess)
				return fmt.Errorf("tools.Execute (%s): %w", tc.Function.Name, err)
			}
			sess.Add(toolResp)
		}
	}

	sess.Add(provider.Message{
		Role:    provider.RoleUser,
		Content: "[Max iterations reached]",
	})

	_ = w.sessionStore.Save(sess)
	return fmt.Errorf("max iterations (%d) reached", maxIterations)
}

// SwitchSession changes the current session to an existing one.
// If Run() is active, it will be cancelled and waited for before switching.
func (w *Workflow) SwitchSession(id string) error {
	// Cancel any running loop and wait for it to finish
	w.mu.Lock()
	cancel := w.runCancel
	done := w.runDone
	w.mu.Unlock()

	if cancel != nil {
		cancel()
		<-done
	}

	s, err := w.sessionStore.Get(id)
	if err != nil {
		return err
	}
	w.currentSession = s
	return nil
}

// NewSession creates a new session and sets it as current.
// If Run() is active, it will be cancelled and waited for before switching.
func (w *Workflow) NewSession() error {
	// Cancel any running loop and wait for it to finish
	w.mu.Lock()
	cancel := w.runCancel
	done := w.runDone
	w.mu.Unlock()

	if cancel != nil {
		cancel()
		<-done
	}

	s, err := w.sessionStore.Create()
	if err != nil {
		return err
	}
	w.currentSession = s
	return nil
}

// DeleteSession removes a session by ID.
// If Run() is active on the session being deleted, it will be cancelled first.
func (w *Workflow) DeleteSession(id string) error {
	// Cancel any running loop on this session and wait for it to finish
	if w.currentSession != nil && w.currentSession.ID() == id {
		w.mu.Lock()
		cancel := w.runCancel
		done := w.runDone
		w.mu.Unlock()

		if cancel != nil {
			cancel()
			<-done
		}
	}

	if err := w.sessionStore.Delete(id); err != nil {
		return err
	}
	if w.currentSession != nil && w.currentSession.ID() == id {
		w.currentSession = nil
	}
	return nil
}

// CurrentSession returns the currently active session.
func (w *Workflow) CurrentSession() *session.Session {
	return w.currentSession
}

// ListSessions returns summaries of all available sessions.
func (w *Workflow) ListSessions() ([]session.SessionSummary, error) {
	return w.sessionStore.List()
}

// SetModel sets the current model for LLM requests.
func (w *Workflow) SetModel(model string) {
	w.currentModel = model
}

// ListModels returns available models from the provider.
func (w *Workflow) ListModels(ctx context.Context) ([]string, error) {
	return w.provider.ListModels(ctx)
}
