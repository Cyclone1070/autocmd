package workflow

import (
	"fmt"

	"github.com/Cyclone1070/iav/internal/domain"
)

// historySessionStore defines the subset of session store operations needed
// by the history workflow.
type historySessionStore interface {
	Get(id string) (*domain.Session, error)
	List() ([]domain.SessionSummary, error)
}

// historyStateStore defines the subset of state operations needed
// by the history workflow.
type historyStateStore interface {
	CurrentSessionID() string
}

// HistoryDeps contains the dependencies required to run the history workflow.
type HistoryDeps struct {
	Store historySessionStore
	State historyStateStore
}

// HistoryResult is the outcome of a successful history workflow run.
type HistoryResult struct {
	Session *domain.Session
}

// ResolveSession resolves which session's history should be displayed and loads it.
func ResolveSession(deps *HistoryDeps, argSessionID string) (*HistoryResult, error) {
	if deps == nil || deps.Store == nil || deps.State == nil {
		return nil, fmt.Errorf("invalid history dependencies")
	}

	// 1. Prefer the explicit argument if present.
	sessionID := argSessionID

	// 2. Fallback to the current session ID in state.
	if sessionID == "" {
		sessionID = deps.State.CurrentSessionID()
	}

	// 3. As a last resort, pick the first session from the store, if any.
	if sessionID == "" {
		summaries, err := deps.Store.List()
		if err != nil || len(summaries) == 0 {
			return nil, fmt.Errorf("no current session found and no history available")
		}
		sessionID = summaries[0].ID
	}

	sess, err := deps.Store.Get(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load session %s: %w", sessionID, err)
	}

	return &HistoryResult{Session: sess}, nil
}

