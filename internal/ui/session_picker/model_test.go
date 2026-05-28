package session_picker

import (
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

type mockPathResolver struct {
	displayPath func(string) string
}

func (m *mockPathResolver) DisplayPath(path string) string {
	if m.displayPath != nil {
		return m.displayPath(path)
	}
	return path
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
		m := NewModel(bus, theme, &mockPathResolver{})
		assert.Empty(t, m.View())
	})

	t.Run("Snapshot received -> List view", func(t *testing.T) {
		bus := new(mockBus)
		m := NewModel(bus, theme, &mockPathResolver{})

		m.Update(result)

		assert.Contains(t, m.View(), "SESSIONS")
		assert.Contains(t, m.View(), "Session 1")
	})

	t.Run("Snapshot with blank name shows Untitled label", func(t *testing.T) {
		bus := new(mockBus)
		m := NewModel(bus, theme, &mockPathResolver{})
		blank := domain.SessionListEvent{
			Sessions:         []domain.SessionSummary{{ID: "s-new", Name: ""}},
			CurrentSessionID: "s-new",
		}

		m.Update(blank)
		assert.Contains(t, m.View(), "Untitled")
		assert.NotContains(t, m.View(), "(untitled)")
	})

	t.Run("Create session: 'n'", func(t *testing.T) {
		bus := new(mockBus)
		bus.On("SendAction", domain.CreateSessionAction{}).Return()

		m := NewModel(bus, theme, &mockPathResolver{})
		m.Update(result)

		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
		assert.Nil(t, cmd)
		assert.Equal(t, "Untitled", m.selectedName)
		bus.AssertExpectations(t)
	})

	t.Run("Rename session start: 'r'", func(t *testing.T) {
		bus := new(mockBus)
		m := NewModel(bus, theme, &mockPathResolver{})
		m.Update(result)

		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
		assert.Contains(t, m.View(), "Rename session")
	})

	t.Run("Rename session submit: enter", func(t *testing.T) {
		bus := new(mockBus)
		bus.On("SendAction", domain.RenameSessionAction{ID: "s1", Name: "Edited"}).Return()

		m := NewModel(bus, theme, &mockPathResolver{})
		m.Update(result)

		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
		m.textInput.SetValue("Edited")

		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		bus.AssertExpectations(t)
	})

	t.Run("Delete session: 'd'", func(t *testing.T) {
		bus := new(mockBus)
		bus.On("SendAction", domain.DeleteSessionAction{ID: "s1"}).Return()

		m := NewModel(bus, theme, &mockPathResolver{})
		m.Update(result)

		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
		bus.AssertExpectations(t)
	})

	t.Run("Select session: 'enter'", func(t *testing.T) {
		bus := new(mockBus)
		bus.On("SendAction", domain.SelectSessionAction{ID: "s1"}).Return()

		m := NewModel(bus, theme, &mockPathResolver{})
		m.Update(result)

		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		bus.AssertExpectations(t)
	})

	t.Run("Select session: 'space' (hidden shortcut)", func(t *testing.T) {
		bus := new(mockBus)
		bus.On("SendAction", domain.SelectSessionAction{ID: "s1"}).Return()

		m := NewModel(bus, theme, &mockPathResolver{})
		m.Update(result)

		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
		assert.Nil(t, cmd, "space select should dispatch action, not quit directly")
		bus.AssertExpectations(t)
	})

	t.Run("DoneEvent -> Quitting", func(t *testing.T) {
		bus := new(mockBus)
		m := NewModel(bus, theme, &mockPathResolver{})
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
		m := NewModel(bus, theme, &mockPathResolver{})
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

		m := NewModel(bus, theme, &mockPathResolver{})
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

	t.Run("Group header uses pathResolver.DisplayPath", func(t *testing.T) {
		bus := new(mockBus)
		pr := &mockPathResolver{
			displayPath: func(p string) string {
				if p == "/home/user/repos/projectA" {
					return "~/repos/projectA"
				}
				return p
			},
		}
		m := NewModel(bus, theme, pr)

		event := domain.SessionListEvent{
			Sessions: []domain.SessionSummary{
				{
					ID:         "s1",
					Name:       "Session 1",
					WorkingDir: "/home/user/repos/projectA",
				},
			},
			CurrentSessionID: "",
		}

		m.initializePicker(&event)
		require.NotNil(t, m.picker)

		item, ok := m.picker.CursorItem()
		require.True(t, ok)
		assert.Equal(t, "~/repos/projectA", item.Group)
	})

	t.Run("Cross-directory sessions are faded", func(t *testing.T) {
		bus := new(mockBus)
		m := NewModel(bus, theme, &mockPathResolver{})

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

	t.Run("DoneEvent with current-directory session succeeds", func(t *testing.T) {
		bus := new(mockBus)
		bus.On("SendAction", domain.SelectSessionAction{ID: "s2"}).Return()
		m := NewModel(bus, theme, &mockPathResolver{})

		event := domain.SessionListEvent{
			WorkingDir: "/current-dir",
			Sessions: []domain.SessionSummary{
				{
					ID:         "s2",
					Name:       "Session 2",
					WorkingDir: "/current-dir",
				},
			},
			CurrentSessionID: "",
		}

		m.Update(event)

		// Select the session
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		assert.Equal(t, "s2", m.selectedID)

		// Receive SessionSelectedEvent
		m.Update(domain.SessionSelectedEvent{
			ID:             "s2",
			SwitchRequired: false,
			TargetDir:      "",
		})

		// Receive DoneEvent
		_, cmd := m.Update(domain.DoneEvent{})
		assert.NotNil(t, cmd)
		assert.True(t, m.quitting)
	})

	t.Run("DoneEvent with cross-directory session uses DisplayPath", func(t *testing.T) {
		bus := new(mockBus)
		bus.On("SendAction", domain.SelectSessionAction{ID: "s2"}).Return()

		var calledWith string
		pr := &mockPathResolver{
			displayPath: func(p string) string {
				calledWith = p
				return "~/repos/334-Repository"
			},
		}
		m := NewModel(bus, theme, pr)

		event := domain.SessionListEvent{
			WorkingDir: "/home/user/repos/current",
			Sessions: []domain.SessionSummary{
				{
					ID:         "s2",
					Name:       "Session 2",
					WorkingDir: "/home/user/repos/334-Repository",
				},
			},
			CurrentSessionID: "",
		}

		m.Update(event)
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		assert.Equal(t, "s2", m.selectedID)

		m.Update(domain.SessionSelectedEvent{
			ID:             "s2",
			SwitchRequired: true,
			TargetDir:      "/home/user/repos/334-Repository",
		})

		_, cmd := m.Update(domain.DoneEvent{})
		assert.NotNil(t, cmd)
		assert.True(t, m.quitting)
		assert.Equal(t, "/home/user/repos/334-Repository", calledWith, "should delegate to pathResolver.DisplayPath")
	})
}

