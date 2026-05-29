package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type fakeHistoryStore struct {
	listErr   error
	getErr    error
	sessions  map[string]*domain.Session
	summaries []domain.SessionMetadata
}

func (f *fakeHistoryStore) GetSession(id string) (*domain.Session, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if s, ok := f.sessions[id]; ok {
		return s, nil
	}
	return nil, errors.New("not found")
}

func (f *fakeHistoryStore) List() ([]domain.SessionMetadata, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.summaries, nil
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
			"s1": {SessionMetadata: domain.SessionMetadata{ID: "s1"}, SessionMessages: domain.SessionMessages{Messages: []*schema.Message{{Role: schema.User, Content: "hi"}}}},
		},
	}
	bus := new(mockHistoryBus)

	bus.On("SendUIUpdate", mock.MatchedBy(func(ev domain.UIUpdate) bool {
		snapshot, ok := ev.(domain.HistoryEvent)
		return ok && len(snapshot.Messages) == 1
	})).Return()
	bus.On("SendUIUpdate", domain.DoneEvent{}).Return()

	done := RunHistory(ctx, &HistoryDeps{
		Store:     store,
		SessionID: "s1",
	}, bus)

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("workflow timed out")
	}

	bus.AssertExpectations(t)
}

