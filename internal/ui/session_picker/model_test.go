package session_picker

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockBus struct {
	mock.Mock
}

func (m *mockBus) UIUpdates() <-chan domain.UIUpdate {
	args := m.Called()
	return args.Get(0).(<-chan domain.UIUpdate)
}

func (m *mockBus) SendAction(act domain.Action) {
	m.Called(act)
}

func TestSessionPickerUI(t *testing.T) {
	summaries := []domain.SessionSummary{
		{ID: "s1", Name: "Session 1"},
	}
	result := domain.SessionListEvent{
		Sessions:         summaries,
		CurrentSessionID: "s1",
	}
	theme := ui.NewTheme(ui.ThemeConfig{})

	t.Run("Initial view is empty", func(t *testing.T) {
		bus := new(mockBus)
		m := NewModel(bus, theme)
		assert.Empty(t, m.View())
	})

	t.Run("Snapshot received -> List view", func(t *testing.T) {
		bus := new(mockBus)
		m := NewModel(bus, theme)
		
		m.Update(result)

		assert.Contains(t, m.View(), "SESSIONS")
		assert.Contains(t, m.View(), "Session 1")
	})

	t.Run("Create session: 'n'", func(t *testing.T) {
		bus := new(mockBus)
		bus.On("SendAction", domain.CreateSessionAction{}).Return()
		
		m := NewModel(bus, theme)
		m.Update(result)

		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
		assert.Nil(t, cmd)
		assert.Equal(t, "(new session)", m.selectedName)
		bus.AssertExpectations(t)
	})

	t.Run("Rename session start: 'r'", func(t *testing.T) {
		bus := new(mockBus)
		m := NewModel(bus, theme)
		m.Update(result)

		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
		assert.Contains(t, m.View(), "Rename session")
	})

	t.Run("Rename session submit: enter", func(t *testing.T) {
		bus := new(mockBus)
		bus.On("SendAction", domain.RenameSessionAction{ID: "s1", Name: "Edited"}).Return()
		
		m := NewModel(bus, theme)
		m.Update(result)

		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
		m.textInput.SetValue("Edited")
		
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		bus.AssertExpectations(t)
	})

	t.Run("Delete session: 'd'", func(t *testing.T) {
		bus := new(mockBus)
		bus.On("SendAction", domain.DeleteSessionAction{ID: "s1"}).Return()
		
		m := NewModel(bus, theme)
		m.Update(result)

		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
		bus.AssertExpectations(t)
	})

	t.Run("Select session: 'enter'", func(t *testing.T) {
		bus := new(mockBus)
		bus.On("SendAction", domain.SelectSessionAction{ID: "s1"}).Return()
		
		m := NewModel(bus, theme)
		m.Update(result)

		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		bus.AssertExpectations(t)
	})
	
	t.Run("DoneEvent -> Quitting", func(t *testing.T) {
		bus := new(mockBus)
		m := NewModel(bus, theme)
		m.Update(result)

		m.Update(domain.DoneEvent{})
		assert.Contains(t, m.selectedName, "Session 1")
		assert.Empty(t, m.View())
	})
}
