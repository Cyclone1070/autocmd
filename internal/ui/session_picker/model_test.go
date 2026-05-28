package session_picker

import (
	"os"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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
	theme := &ui.Theme{}

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

	t.Run("Snapshot with blank name shows new session label", func(t *testing.T) {
		bus := new(mockBus)
		m := NewModel(bus, theme)
		blank := domain.SessionListEvent{
			Sessions:         []domain.SessionSummary{{ID: "s-new", Name: ""}},
			CurrentSessionID: "s-new",
		}

		m.Update(blank)
		assert.Contains(t, m.View(), "(new session)")
		assert.NotContains(t, m.View(), "(untitled)")
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

	t.Run("Select session: 'space' (hidden shortcut)", func(t *testing.T) {
		bus := new(mockBus)
		bus.On("SendAction", domain.SelectSessionAction{ID: "s1"}).Return()

		m := NewModel(bus, theme)
		m.Update(result)

		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
		assert.Nil(t, cmd, "space select should dispatch action, not quit directly")
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

	t.Run("StopAction on 'q'", func(t *testing.T) {
		bus := new(mockBus)
		bus.On("SendAction", domain.StopAction{}).Return()
		ch := make(chan domain.UIUpdate, 1)
		ch <- domain.DoneEvent{}
		close(ch)
		bus.On("UIUpdates").Return((<-chan domain.UIUpdate)(ch))
		m := NewModel(bus, theme)
		m.Update(result)

		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})

		assert.NotNil(t, cmd, "cancel should keep polling for DoneEvent (not quit immediately)")
		msg := cmd()
		_, ok := msg.(domain.DoneEvent)
		assert.True(t, ok, "cancel should schedule pollBus (next msg should be DoneEvent)")
		bus.AssertCalled(t, "SendAction", domain.StopAction{})
	})

	t.Run("CancelRequested ignores subsequent SessionListEvent until DoneEvent", func(t *testing.T) {
		bus := new(mockBus)
		bus.On("SendAction", domain.StopAction{}).Return()

		ch1 := make(chan domain.UIUpdate, 1)
		ch1 <- domain.DoneEvent{}
		close(ch1)
		bus.On("UIUpdates").Return((<-chan domain.UIUpdate)(ch1)).Once()

		m := NewModel(bus, theme)
		m.Update(result)
		assert.NotNil(t, m.picker)

		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
		assert.True(t, m.cancelRequested)

		prevPicker := m.picker
		ch2 := make(chan domain.UIUpdate, 1)
		ch2 <- domain.SessionListEvent{Sessions: []domain.SessionSummary{{ID: "sx", Name: "X"}}, CurrentSessionID: ""}
		close(ch2)
		bus.On("UIUpdates").Return((<-chan domain.UIUpdate)(ch2)).Once()

		_, cmd := m.Update(domain.SessionListEvent{Sessions: []domain.SessionSummary{{ID: "sx", Name: "X"}}, CurrentSessionID: ""})
		assert.NotNil(t, cmd)
		_ = cmd()
		assert.Equal(t, prevPicker, m.picker)
	})

	t.Run("Home directory path is replaced with ~", func(t *testing.T) {
		home, err := os.UserHomeDir()
		require.NoError(t, err)

		bus := new(mockBus)
		m := NewModel(bus, theme)

		targetDir := home + "/repos/projectA"
		event := domain.SessionListEvent{
			Sessions: []domain.SessionSummary{
				{
					ID:         "s1",
					Name:       "Session 1",
					WorkingDir: targetDir,
				},
			},
			CurrentSessionID: "",
		}

		m.initializePicker(&event)
		require.NotNil(t, m.picker)

		// Get item from picker and check Group field
		item, ok := m.picker.CursorItem()
		require.True(t, ok)
		assert.Equal(t, "~/repos/projectA", item.Group)
	})

	t.Run("Cross-directory sessions are faded", func(t *testing.T) {
		bus := new(mockBus)
		m := NewModel(bus, theme)

		event := domain.SessionListEvent{
			WorkingDir: "/current-dir",
			Sessions: []domain.SessionSummary{
				{
					ID:         "s1",
					Name:       "Session 1",
					WorkingDir: "/current-dir",
				},
				{
					ID:         "s2",
					Name:       "Session 2",
					WorkingDir: "/other-dir",
				},
			},
			CurrentSessionID: "s1",
		}

		m.initializePicker(&event)
		require.NotNil(t, m.picker)

		// Cursor is at s1 (current dir), should not be faded
		item1, ok := m.picker.CursorItem()
		require.True(t, ok)
		assert.False(t, item1.Faded)

		// Move cursor to s2, should be faded
		m.picker.Update(tea.KeyMsg{Type: tea.KeyDown})
		item2, ok := m.picker.CursorItem()
		require.True(t, ok)
		assert.True(t, item2.Faded)
	})
}

