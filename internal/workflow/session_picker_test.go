package workflow

import (
	"context"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockSessionPickerStore struct {
	mock.Mock
}

func (m *mockSessionPickerStore) List() ([]domain.SessionSummary, error) {
	args := m.Called()
	return args.Get(0).([]domain.SessionSummary), args.Error(1)
}

func (m *mockSessionPickerStore) Create() (*domain.Session, error) {
	args := m.Called()
	return args.Get(0).(*domain.Session), args.Error(1)
}

func (m *mockSessionPickerStore) Rename(id, name string) error {
	args := m.Called(id, name)
	return args.Error(0)
}

func (m *mockSessionPickerStore) Delete(id string) error {
	args := m.Called(id)
	return args.Error(0)
}

type mockSessionPickerState struct {
	mock.Mock
}

func (m *mockSessionPickerState) CurrentSessionID() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockSessionPickerState) SetCurrentSessionID(id string) {
	m.Called(id)
}

func (m *mockSessionPickerState) Save() error {
	args := m.Called()
	return args.Error(0)
}

func TestSessionPickerWorkflow(t *testing.T) {
	ctx := context.Background()
	summaries := []domain.SessionSummary{
		{ID: "s1", Name: "Session 1"},
		{ID: "s2", Name: "Session 2"},
	}

	t.Run("prepareSelection returns summaries and current ID", func(t *testing.T) {
		store := new(mockSessionPickerStore)
		state := new(mockSessionPickerState)

		store.On("List").Return(summaries, nil)
		state.On("CurrentSessionID").Return("s1")

		wf := newSessionPickerWorkflow(store, state)
		res, err := wf.prepareSelection(ctx)

		assert.NoError(t, err)
		assert.Equal(t, summaries, res.Sessions)
		assert.Equal(t, "s1", res.CurrentSessionID)
		store.AssertExpectations(t)
		state.AssertExpectations(t)
	})

	t.Run("applySelection updates state", func(t *testing.T) {
		store := new(mockSessionPickerStore)
		state := new(mockSessionPickerState)

		state.On("SetCurrentSessionID", "s2").Return()
		state.On("Save").Return(nil)

		wf := newSessionPickerWorkflow(store, state)
		err := wf.applySelection(ctx, "s2")

		assert.NoError(t, err)
		state.AssertExpectations(t)
	})

	t.Run("createSession creates and updates state", func(t *testing.T) {
		store := new(mockSessionPickerStore)
		state := new(mockSessionPickerState)

		newSess := &domain.Session{ID: "new-id"}
		store.On("Create").Return(newSess, nil)
		state.On("SetCurrentSessionID", "new-id").Return()
		state.On("Save").Return(nil)

		wf := newSessionPickerWorkflow(store, state)
		id, err := wf.createSession(ctx)

		assert.NoError(t, err)
		assert.Equal(t, "new-id", id)
		store.AssertExpectations(t)
		state.AssertExpectations(t)
	})

	t.Run("renameSession calls store", func(t *testing.T) {
		store := new(mockSessionPickerStore)
		state := new(mockSessionPickerState)

		store.On("Rename", "s1", "Better Name").Return(nil)

		wf := newSessionPickerWorkflow(store, state)
		err := wf.renameSession(ctx, "s1", "Better Name")

		assert.NoError(t, err)
		store.AssertExpectations(t)
	})

	t.Run("deleteSession calls store and clears state if current", func(t *testing.T) {
		store := new(mockSessionPickerStore)
		state := new(mockSessionPickerState)

		store.On("Delete", "s1").Return(nil)
		state.On("CurrentSessionID").Return("s1")
		state.On("SetCurrentSessionID", "").Return()
		state.On("Save").Return(nil)

		wf := newSessionPickerWorkflow(store, state)
		err := wf.deleteSession(ctx, "s1")

		assert.NoError(t, err)
		store.AssertExpectations(t)
		state.AssertExpectations(t)
	})
}
