package workflow

import (
	"context"
	"errors"

	"github.com/Cyclone1070/iav/internal/domain"
)

// sessionStore defines the persistence operations for chat sessions.
type sessionStore interface {
	Create() (*domain.Session, error)
	Get(id string) (*domain.Session, error)
	Save(s *domain.Session) error
	GenerateName(ctx context.Context, llm domain.LLM, target string) (string, error)
}

type workspaceSessionStore interface {
	FindLatestForDir(dir string) (*domain.SessionSummary, error)
	Create() (*domain.Session, error)
	Get(id string) (*domain.Session, error)
	Save(s *domain.Session) error
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
	Store        sessionStore
	LLM          domain.LLM
	ToolRegistry any // retained for backward compatibility; unused in prompt workflow
	Agent        agentRunner
	Bus          bus
	Forwarder    ActionForwarder
	Session      *domain.Session
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

		_ = deps.Store.Save(sess)

		deps.Bus.SendUIUpdate(domain.DoneEvent{})
		done <- err
	}()

	return done
}

// ResolveWorkspaceSession finds or creates the active session for a working directory.
func ResolveWorkspaceSession(store workspaceSessionStore, workingDir string) (*domain.Session, error) {
	latest, err := store.FindLatestForDir(workingDir)
	if err != nil {
		return nil, err
	}

	if latest != nil {
		sess, err := store.Get(latest.ID)
		if err == nil {
			return sess, nil
		}
	}

	// Create new session scoped to workingDir
	sess, err := store.Create()
	if err != nil {
		return nil, err
	}
	sess.WorkingDir = workingDir
	if err := store.Save(sess); err != nil {
		return nil, err
	}
	return sess, nil
}
