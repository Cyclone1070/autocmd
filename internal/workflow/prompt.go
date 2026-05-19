package workflow

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/Cyclone1070/iav/internal/domain"
)

// sessionStore defines the persistence operations for chat sessions.
type sessionStore interface {
	Create() (*domain.Session, error)
	Get(id string) (*domain.Session, error)
	Save(s *domain.Session) error
	GenerateName(ctx context.Context, llm domain.LLM, target string) (string, error)
}

// stateStore defines the persistence operations for application state.
type stateStore interface {
	CurrentSessionID() string
	SetCurrentSessionID(string)
	Save() error
}

type agentRunner interface {
	Run(ctx context.Context, sess *domain.Session, input string) error
}

// ActionForwarder defines the interface for delivering structured actions.
type ActionForwarder interface {
	Deliver(domain.Action)
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
	ToolRegistry any // retained for backward compatibility; unused in prompt workflow
	Agent        agentRunner
	Bus          bus
	// Forwarder is used to route structured tool actions (like QuestionAnswerAction)
	// to the blocked tool executor.
	Forwarder ActionForwarder
}

// RunPrompt executes the full agentic flow: session resolution, agent loop,
// and UI synchronization. It returns a channel that will receive the result
// when the agent loop completes.
func RunPrompt(ctx context.Context, input string, deps *PromptDeps) <-chan error {
	done := make(chan error, 1)

	sessionID := deps.State.CurrentSessionID()

	var sess *domain.Session
	var err error
	if sessionID == "" {
		sess, err = deps.Store.Create()
		if err != nil {
			done <- err
			return done
		}
		deps.State.SetCurrentSessionID(sess.ID)
		if err := deps.State.Save(); err != nil {
			slog.Warn("failed to save state", "error", err)
		}
	} else {
		sess, err = deps.Store.Get(sessionID)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				slog.Warn("current session not found, creating new one", "session_id", sessionID)
				sess, err = deps.Store.Create()
				if err != nil {
					done <- err
					return done
				}
				deps.State.SetCurrentSessionID(sess.ID)
				if err := deps.State.Save(); err != nil {
					slog.Warn("failed to save state", "error", err)
				}
			} else {
				done <- err
				return done
			}
		} else if deps.State.CurrentSessionID() != sess.ID {
			deps.State.SetCurrentSessionID(sess.ID)
			if err := deps.State.Save(); err != nil {
				slog.Warn("failed to save state", "error", err)
			}
		}
	}

	workflowCtx, cancel := context.WithCancel(ctx)
	workflowCtx = domain.WithSessionID(workflowCtx, sess.ID)

	nameChan := make(chan string, 1)
	if sess.Name == "" {
		// Capture first message content safely for naming
		var target string
		if len(sess.Messages) > 0 {
			target = sess.Messages[0].Content
		}
		if target == "" {
			target = input
		}

		go func() {
			defer close(nameChan)
			name, err := deps.Store.GenerateName(workflowCtx, deps.LLM, target)
			if err == nil {
				nameChan <- name
			}
		}()
	} else {
		// Session already has a name; ensure receivers don't block waiting for naming.
		close(nameChan)
	}

	// Monitor for workflow actions (e.g. StopAction, question answers)
	go func() {
		for act := range deps.Bus.WorkflowActions() {
			switch act.(type) {
			case domain.StopAction:
				cancel()
			default:
				if deps.Forwarder != nil {
					deps.Forwarder.Deliver(act)
				}
			}
		}
	}()

	go func() {
		defer cancel()
		deps.Bus.SendUIUpdate(domain.ConnectingEvent{})
		err := deps.Agent.Run(workflowCtx, sess, input)

		select {
		case name, ok := <-nameChan:
			if ok {
				sess.Name = name
			}
		default:
			deps.Bus.SendUIUpdate(domain.WaitingForNamingEvent{})
			if name, ok := <-nameChan; ok {
				sess.Name = name
			}
		}

		_ = deps.Store.Save(sess)

		deps.Bus.SendUIUpdate(domain.DoneEvent{})
		done <- err
	}()

	return done
}
