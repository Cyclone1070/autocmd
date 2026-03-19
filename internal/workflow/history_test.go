package workflow

import (
	"errors"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
)

type fakeHistoryStore struct {
	sessions  map[string]*domain.Session
	summaries []domain.SessionSummary
	listErr   error
	getErr    error
}

func (f *fakeHistoryStore) Get(id string) (*domain.Session, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if s, ok := f.sessions[id]; ok {
		return s, nil
	}
	return nil, errors.New("not found")
}

func (f *fakeHistoryStore) List() ([]domain.SessionSummary, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.summaries, nil
}

type fakeHistoryState struct {
	currentID string
}

func (f *fakeHistoryState) CurrentSessionID() string {
	return f.currentID
}

func TestRunHistory_NoArgProvided(t *testing.T) {
	store := &fakeHistoryStore{
		sessions: map[string]*domain.Session{
			"state-id": {ID: "state-id"},
		},
	}
	state := &fakeHistoryState{currentID: "state-id"}

	res, err := ResolveSession(&HistoryDeps{
		Store: store,
		State: state,
	})
	if err != nil {
		t.Fatalf("RunHistory returned error: %v", err)
	}
	if res.Session == nil || res.Session.ID != "state-id" {
		t.Fatalf("expected session ID %q, got %#v", "state-id", res.Session)
	}
}

func TestRunHistory_WithCurrentSessionID(t *testing.T) {
	store := &fakeHistoryStore{
		sessions: map[string]*domain.Session{
			"state-id": {ID: "state-id"},
		},
	}
	state := &fakeHistoryState{currentID: "state-id"}

	res, err := ResolveSession(&HistoryDeps{
		Store: store,
		State: state,
	})
	if err != nil {
		t.Fatalf("RunHistory returned error: %v", err)
	}
	if res.Session == nil || res.Session.ID != "state-id" {
		t.Fatalf("expected session ID %q, got %#v", "state-id", res.Session)
	}
}

func TestRunHistory_NoCurrentSessionInState(t *testing.T) {
	store := &fakeHistoryStore{
		sessions: map[string]*domain.Session{
			"some-id": {ID: "some-id"},
		},
		summaries: []domain.SessionSummary{
			{ID: "some-id"},
		},
	}
	state := &fakeHistoryState{currentID: ""}

	_, err := ResolveSession(&HistoryDeps{
		Store: store,
		State: state,
	})
	if err == nil {
		t.Fatalf("expected error when no current session ID is in state")
	}
}

func TestRunHistory_SessionInStateButNotFoundInStore(t *testing.T) {
	store := &fakeHistoryStore{
		sessions: map[string]*domain.Session{},
	}
	state := &fakeHistoryState{currentID: "missing-id"}

	_, err := ResolveSession(&HistoryDeps{
		Store: store,
		State: state,
	})
	if err == nil {
		t.Fatalf("expected error when state session ID is not found in store")
	}
}

