package workflow

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/Cyclone1070/iav/internal/agent"
	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/session"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/Cyclone1070/iav/internal/ui/loop"
	tea "github.com/charmbracelet/bubbletea"
)

// sessionStore defines the persistence operations for chat sessions.
type sessionStore interface {
	Create() (*domain.Session, error)
	Get(id string) (*domain.Session, error)
	Save(s *domain.Session) error
}

type toolRegistry interface {
	Declarations() []domain.Declaration
	Get(name string) (domain.Tool, bool)
}

type programRunner interface {
	Run(m tea.Model) error
}

// PromptDeps contains the dependencies required to run the agent prompt workflow.
type PromptDeps struct {
	Config       *config.Config
	State        *state.State
	Store        sessionStore
	LLM          domain.LLM
	ToolRegistry toolRegistry
	Runner       programRunner
}

// RunPrompt executes the full agentic flow: session resolution, agent loop, 
// and UI synchronization.
func RunPrompt(ctx context.Context, input string, deps *PromptDeps) error {
	var sessionID string
	// For the main agent command, we always use the current session from state.
	sessionID = deps.State.CurrentSessionID

	var sess *domain.Session
	var err error
	if sessionID == "" {
		sess, err = deps.Store.Create()
		if err != nil {
			return err
		}
		deps.State.CurrentSessionID = sess.ID
		if err := state.Save(deps.State); err != nil {
			slog.Warn("failed to save state", "error", err)
		}
	} else {
		sess, err = deps.Store.Get(sessionID)
		if err != nil {
			return err
		}
		if deps.State.CurrentSessionID != sess.ID {
			deps.State.CurrentSessionID = sess.ID
			if err := state.Save(deps.State); err != nil {
				slog.Warn("failed to save state", "error", err)
			}
		}
	}

	events := make(chan domain.Event, 100)
	broker := NewEventBroker(events)
	agentLoop := agent.NewLoop(deps.LLM, deps.ToolRegistry, deps.Config, broker)

	var namingWg sync.WaitGroup
	// Trigger auto-naming if this is a new session (no name yet)
	if sess.Name == "" {
		namingWg.Add(1)
		go func() {
			defer namingWg.Done()
			name, err := session.GenerateName(ctx, deps.LLM, sess, input)
			if err == nil {
				sess.Name = name
			}
		}()
	}

	m := loop.NewModel(events, deps.Config.UI)

	done := make(chan error, 1)
	go func() {
		err := agentLoop.Run(ctx, sess, input)
		namingWg.Wait()

		_ = deps.Store.Save(sess)

		broker.Close()
		select {
		case events <- domain.DoneEvent{}:
		default:
		}
		close(events)
		done <- err
	}()

	if err := deps.Runner.Run(m); err != nil {
		fmt.Fprintf(os.Stderr, "UI failed: %v\n", err)
	}

	agentErr := <-done
	if agentErr != nil && !errors.Is(agentErr, context.Canceled) {
		return fmt.Errorf("agent failed: %w", agentErr)
	}

	return nil
}
