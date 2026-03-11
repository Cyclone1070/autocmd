package workflow

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/Cyclone1070/iav/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

// sessionStore defines the persistence operations for chat sessions.
type sessionStore interface {
	Create() (*domain.Session, error)
	Get(id string) (*domain.Session, error)
	Save(s *domain.Session) error
	GenerateName(ctx context.Context, llm domain.LLM, sess *domain.Session, input string) (string, error)
}

// stateStore defines the persistence operations for application state.
type stateStore interface {
	CurrentSessionID() string
	SetCurrentSessionID(string)
	Save() error
}

type toolRegistry interface {
	Declarations() []domain.Declaration
	Get(name string) (domain.Tool, bool)
}

type agentRunner interface {
	Run(ctx context.Context, sess *domain.Session, input string) error
}

type programRunner interface {
	Run(m tea.Model) error
}

// bus defines the interface for the event bus as used by the prompt workflow.
type bus interface {
	WorkflowActions() <-chan domain.Action
	SendUIUpdate(domain.UIUpdate)
}

// PromptDeps contains the dependencies required to run the agent prompt workflow.
type PromptDeps struct {
	State        stateStore
	Store        sessionStore
	LLM          domain.LLM
	ToolRegistry toolRegistry
	Runner       programRunner
	Agent        agentRunner
	UI           tea.Model
	Bus          bus
}

// RunPrompt executes the full agentic flow: session resolution, agent loop, 
// and UI synchronization.
func RunPrompt(ctx context.Context, input string, deps *PromptDeps) error {
	var sessionID string
	// For the main agent command, we always use the current session from state.
	sessionID = deps.State.CurrentSessionID()

	var sess *domain.Session
	var err error
	if sessionID == "" {
		sess, err = deps.Store.Create()
		if err != nil {
			return err
		}
		deps.State.SetCurrentSessionID(sess.ID)
		if err := deps.State.Save(); err != nil {
			slog.Warn("failed to save state", "error", err)
		}
	} else {
		sess, err = deps.Store.Get(sessionID)
		if err != nil {
			return err
		}
		if deps.State.CurrentSessionID() != sess.ID {
			deps.State.SetCurrentSessionID(sess.ID)
			if err := deps.State.Save(); err != nil {
				slog.Warn("failed to save state", "error", err)
			}
		}
	}

	// Create a cancellable context for the workflow
	workflowCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	
	var namingWg sync.WaitGroup
	// Trigger auto-naming if this is a new session (no name yet)
	if sess.Name == "" {
		namingWg.Add(1)
		go func() {
			defer namingWg.Done()
			name, err := deps.Store.GenerateName(workflowCtx, deps.LLM, sess, input)
			if err == nil {
				sess.Name = name
			}
		}()
	}

	// Monitor for workflow actions (e.g. StopAction)
	go func() {
		for act := range deps.Bus.WorkflowActions() {
			switch act.(type) {
			case domain.StopAction:
				cancel()
			}
		}
	}()

	done := make(chan error, 1)
	go func() {
		err := deps.Agent.Run(workflowCtx, sess, input)
		namingWg.Wait()

		_ = deps.Store.Save(sess)

		deps.Bus.SendUIUpdate(domain.DoneEvent{})
		done <- err
	}()

	if err := deps.Runner.Run(deps.UI); err != nil {
		fmt.Fprintf(os.Stderr, "UI failed: %v\n", err)
	}

	agentErr := <-done
	if agentErr != nil && !errors.Is(agentErr, context.Canceled) {
		return fmt.Errorf("agent failed: %w", agentErr)
	}

	return nil
}
