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

func TestRunHistory_WithArgSessionID(t *testing.T) {
	store := &fakeHistoryStore{
		sessions: map[string]*domain.Session{
			"arg-id": {ID: "arg-id"},
		},
	}
	state := &fakeHistoryState{currentID: "state-id"}

	res, err := RunHistory(&HistoryDeps{
		Store: store,
		State: state,
	}, "arg-id")
	if err != nil {
		t.Fatalf("RunHistory returned error: %v", err)
	}
	if res.Session == nil || res.Session.ID != "arg-id" {
		t.Fatalf("expected session ID %q, got %#v", "arg-id", res.Session)
	}
}

func TestRunHistory_WithCurrentSessionID(t *testing.T) {
	store := &fakeHistoryStore{
		sessions: map[string]*domain.Session{
			"state-id": {ID: "state-id"},
		},
	}
	state := &fakeHistoryState{currentID: "state-id"}

	res, err := RunHistory(&HistoryDeps{
		Store: store,
		State: state,
	}, "")
	if err != nil {
		t.Fatalf("RunHistory returned error: %v", err)
	}
	if res.Session == nil || res.Session.ID != "state-id" {
		t.Fatalf("expected session ID %q, got %#v", "state-id", res.Session)
	}
}

func TestRunHistory_FallbackToList(t *testing.T) {
	store := &fakeHistoryStore{
		sessions: map[string]*domain.Session{
			"listed-id": {ID: "listed-id"},
		},
		summaries: []domain.SessionSummary{
			{ID: "listed-id"},
		},
	}
	state := &fakeHistoryState{currentID: ""}

	res, err := RunHistory(&HistoryDeps{
		Store: store,
		State: state,
	}, "")
	if err != nil {
		t.Fatalf("RunHistory returned error: %v", err)
	}
	if res.Session == nil || res.Session.ID != "listed-id" {
		t.Fatalf("expected session ID %q, got %#v", "listed-id", res.Session)
	}
}

func TestRunHistory_NoSessionsAvailable(t *testing.T) {
	store := &fakeHistoryStore{
		sessions:  map[string]*domain.Session{},
		summaries: nil,
	}
	state := &fakeHistoryState{currentID: ""}

	_, err := RunHistory(&HistoryDeps{
		Store: store,
		State: state,
	}, "")
	if err == nil {
		t.Fatalf("expected error when no sessions are available")
	}
}

