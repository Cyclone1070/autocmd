package session_picker

import (
	"context"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockWorkflow struct {
	mock.Mock
}

func (m *mockWorkflow) PrepareSelection(ctx context.Context) (*domain.SessionPickerSnapshot, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.SessionPickerSnapshot), args.Error(1)
}

func (m *mockWorkflow) ApplySelection(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockWorkflow) CreateSession(ctx context.Context) (string, error) {
	args := m.Called(ctx)
	return args.String(0), args.Error(1)
}

func (m *mockWorkflow) RenameSession(ctx context.Context, id, name string) error {
	args := m.Called(ctx, id, name)
	return args.Error(0)
}

func (m *mockWorkflow) DeleteSession(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func TestSessionPickerUI(t *testing.T) {
	summaries := []domain.SessionSummary{
		{ID: "s1", Name: "Session 1"},
	}
	result := &domain.SessionPickerSnapshot{
		Sessions:         summaries,
		CurrentSessionID: "s1",
	}

	t.Run("Initial flow: Loading -> List", func(t *testing.T) {
		wf := new(mockWorkflow)
		wf.On("PrepareSelection", mock.Anything).Return(result, nil)

		m := NewModel(wf)
		assert.Contains(t, m.View(), "Fetching sessions")

		msg := m.Init()()
		m.Update(msg)

		assert.Contains(t, m.View(), "SESSIONS")
		assert.Contains(t, m.View(), "Session 1")
		wf.AssertExpectations(t)
	})

	t.Run("Create session: 'n'", func(t *testing.T) {
		wf := new(mockWorkflow)
		wf.On("CreateSession", mock.Anything).Return("new-id", nil)

		m := NewModel(wf)
		m.Update(prepareResultMsg{data: result})

		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
		assert.NotNil(t, cmd)
		
		msg := cmd() // Returns applyResultMsg
		m.Update(msg) 
		
		wf.AssertExpectations(t)
	})

	t.Run("Rename session: 'r' -> enter", func(t *testing.T) {
		wf := new(mockWorkflow)
		wf.On("RenameSession", mock.Anything, "s1", "Session 1 Edited").Return(nil)
		wf.On("PrepareSelection", mock.Anything).Return(result, nil)

		m := NewModel(wf)
		m.Update(prepareResultMsg{data: result})

		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" Edited")})
		
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		assert.NotNil(t, cmd)
		
		msg := cmd() // returns mutationResultMsg{refresh: true}
		_, cmdRefresh := m.Update(msg)
		assert.NotNil(t, cmdRefresh)
		
		msgRefresh := cmdRefresh() // returns prepareResultMsg
		m.Update(msgRefresh)

		wf.AssertExpectations(t)
	})

	t.Run("Delete session: 'd'", func(t *testing.T) {
		wf := new(mockWorkflow)
		wf.On("DeleteSession", mock.Anything, "s1").Return(nil)
		wf.On("PrepareSelection", mock.Anything).Return(result, nil)

		m := NewModel(wf)
		m.Update(prepareResultMsg{data: result})

		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
		assert.NotNil(t, cmd)
		
		msg := cmd() // returns mutationResultMsg{refresh: true}
		_, cmdRefresh := m.Update(msg)
		assert.NotNil(t, cmdRefresh)
		
		msgRefresh := cmdRefresh() // returns prepareResultMsg
		m.Update(msgRefresh)

		wf.AssertExpectations(t)
	})

	t.Run("Select session: 'enter'", func(t *testing.T) {
		wf := new(mockWorkflow)
		wf.On("ApplySelection", mock.Anything, "s1").Return(nil)

		m := NewModel(wf)
		m.Update(prepareResultMsg{data: result})

		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		assert.NotNil(t, cmd)
		
		msg := cmd() // returns applyResultMsg
		m.Update(msg)

		assert.Empty(t, m.View())
		wf.AssertExpectations(t)
	})
}
