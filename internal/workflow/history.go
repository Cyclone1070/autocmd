package workflow

import (
	"context"
	"fmt"

	"github.com/Cyclone1070/autocmd/internal/domain"
)

// historySessionStore defines the subset of session store operations needed
// by the history workflow.
type historySessionStore interface {
	GetSession(id string) (*domain.Session, error)
}

type historyBus interface {
	SendUIUpdate(domain.UIUpdate)
}

// HistoryDeps contains the dependencies required to run the history workflow.
type HistoryDeps struct {
	Store     historySessionStore
	SessionID string
}

// RunHistory starts the history gathering workflow asynchronously.
func RunHistory(_ context.Context, deps *HistoryDeps, bus historyBus) <-chan error {
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
	if deps == nil || deps.Store == nil {
		return nil, fmt.Errorf("invalid history dependencies")
	}
	if deps.SessionID == "" {
		return &HistoryResult{Session: &domain.Session{}}, nil
	}

	sess, err := deps.Store.GetSession(deps.SessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load session %s: %w", deps.SessionID, err)
	}

	return &HistoryResult{Session: sess}, nil
}
