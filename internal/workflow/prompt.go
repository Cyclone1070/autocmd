package workflow

import (
	"context"
	"errors"

	"github.com/Cyclone1070/iav/internal/domain"
)

// sessionStore defines the persistence operations for sessions.
type sessionStore interface {
	FindActiveForDir(dir string) (*domain.SessionMetadata, error)
	Create(workingDir string) (*domain.Session, error)
	GetSession(id string) (*domain.Session, error)
	SaveSession(s *domain.Session) error
	GenerateName(ctx context.Context, llm domain.LLM, target string) (string, error)
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
	Store     sessionStore
	LLM       domain.LLM
	Agent     agentRunner
	Bus       bus
	Forwarder ActionForwarder
	Session   *domain.Session
}

func RunPrompt(ctx context.Context, input string, deps *PromptDeps) <-chan error {
	done := make(chan error, 1)

	sess := deps.Session
	if sess == nil {
		done <- errors.New("session is required")
		return done
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

		_ = deps.Store.SaveSession(sess)

		deps.Bus.SendUIUpdate(domain.DoneEvent{})
		done <- err
	}()

	return done
}

// ResolveWorkspaceSession finds or creates the active session for a working directory.
func ResolveWorkspaceSession(store sessionStore, workingDir string) (*domain.Session, error) {
	active, err := store.FindActiveForDir(workingDir)
	if err != nil {
		return nil, err
	}
	if active != nil {
		sess, err := store.GetSession(active.ID)
		if err == nil {
			return sess, nil
		}
	}

	// Create new session scoped to workingDir
	sess, err := store.Create(workingDir)
	if err != nil {
		return nil, err
	}
	sess.Active = true
	if err := store.SaveSession(sess); err != nil {
		return nil, err
	}
	return sess, nil
}
