package workflow

import (
	"context"
	"fmt"

	"github.com/Cyclone1070/iav/internal/domain"
)

// historySessionStore defines the subset of session store operations needed
// by the history workflow.
type historySessionStore interface {
	Get(id string) (*domain.Session, error)
}

// historyStateStore defines the subset of state operations needed
// by the history workflow.
type historyStateStore interface {
	CurrentSessionID() string
}

type historyBus interface {
	SendUIUpdate(domain.UIUpdate)
}

// HistoryDeps contains the dependencies required to run the history workflow.
type HistoryDeps struct {
	Store historySessionStore
	State historyStateStore
}

// RunHistory starts the history gathering workflow asynchronously.
func RunHistory(ctx context.Context, deps *HistoryDeps, bus historyBus) <-chan error {
	done := make(chan error, 1)
	go func() {
		defer close(done)
		res, err := ResolveSession(deps)
		if err != nil {
			done <- err
			return
		}

		bus.SendUIUpdate(domain.HistoryEvent{
			Messages:     res.Session.Messages,
			ToolDisplays: res.Session.ToolDisplays,
		})
		bus.SendUIUpdate(domain.DoneEvent{})
		done <- nil
	}()
	return done
}

// HistoryResult is the outcome of a successful history workflow run.
type HistoryResult struct {
	Session *domain.Session
}

// ResolveSession resolves which session's history should be displayed and loads it.
func ResolveSession(deps *HistoryDeps) (*HistoryResult, error) {
	if deps == nil || deps.Store == nil || deps.State == nil {
		return nil, fmt.Errorf("invalid history dependencies")
	}

	sessionID := deps.State.CurrentSessionID()
	if sessionID == "" {
		return nil, fmt.Errorf("no current session found in state")
	}

	sess, err := deps.Store.Get(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load session %s from state: %w", sessionID, err)
	}

	return &HistoryResult{Session: sess}, nil
}
