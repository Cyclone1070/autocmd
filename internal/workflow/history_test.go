package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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

type mockHistoryBus struct {
	mock.Mock
}

func (m *mockHistoryBus) SendUIUpdate(update domain.UIUpdate) {
	m.Called(update)
}

func TestRunHistory(t *testing.T) {
	ctx := context.Background()
	store := &fakeHistoryStore{
		sessions: map[string]*domain.Session{
			"s1": {ID: "s1", Messages: domain.Messages{domain.UserMessage{Content: "hi"}}},
		},
	}
	state := &fakeHistoryState{currentID: "s1"}
	bus := new(mockHistoryBus)

	bus.On("SendUIUpdate", mock.MatchedBy(func(ev domain.UIUpdate) bool {
		snapshot, ok := ev.(domain.HistoryEvent)
		return ok && len(snapshot.Messages) == 1
	})).Return()
	bus.On("SendUIUpdate", domain.DoneEvent{}).Return()

	done := RunHistory(ctx, &HistoryDeps{
		Store: store,
		State: state,
	}, bus)

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("workflow timed out")
	}

	bus.AssertExpectations(t)
}
