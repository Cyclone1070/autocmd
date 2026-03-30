package model_picker

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

func TestModelSelection(t *testing.T) {
	result := domain.ModelListEvent{
		Models: []domain.LLMInfo{
			{ID: "m1", DisplayName: "Model 1"},
			{ID: "m2", DisplayName: "Model 2"},
		},
		ActiveModelID: "m1",
	}
	theme := ui.NewTheme(ui.ThemeConfig{})

	t.Run("Selection triggers SelectModelAction", func(t *testing.T) {
		bus := new(mockBus)
		bus.On("SendAction", domain.SelectModelAction{ID: "m2"}).Return()
		
		m := NewModel(bus, theme)
		
		// Initial state
		assert.Empty(t, m.View())

		// Receive data
		m.Update(result)
		assert.Contains(t, m.View(), "MODELS")
		
		// Press Enter on item
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		
		assert.Nil(t, cmd, "Keypress should not trigger new poller")
		assert.Equal(t, "Model 2", m.selectedName)
		bus.AssertExpectations(t)
	})

	t.Run("Escape sends StopAction and Quits", func(t *testing.T) {
		bus := new(mockBus)
		bus.On("SendAction", domain.StopAction{}).Return()
		ch := make(chan domain.UIUpdate, 1)
		ch <- domain.DoneEvent{}
		close(ch)
		bus.On("UIUpdates").Return((<-chan domain.UIUpdate)(ch))
		
		m := NewModel(bus, theme)
		m.Update(result)

		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		assert.NotNil(t, cmd, "cancel should keep polling for DoneEvent (not quit immediately)")
		msg := cmd()
		_, ok := msg.(domain.DoneEvent)
		assert.True(t, ok, "cancel should schedule pollBus (next msg should be DoneEvent)")
		assert.Equal(t, "", m.selectedName)
		bus.AssertExpectations(t)
	})

	t.Run("DoneEvent triggers Quit", func(t *testing.T) {
		bus := new(mockBus)
		m := NewModel(bus, theme)
		m.Update(result)
		m.selectedName = "Model 1"

		_, cmd := m.Update(domain.DoneEvent{})
		assert.NotNil(t, cmd)
		assert.True(t, m.quitting)
	})

	t.Run("CancelRequested ignores subsequent ModelListEvent until DoneEvent", func(t *testing.T) {
		bus := new(mockBus)
		bus.On("SendAction", domain.StopAction{}).Return()

		ch1 := make(chan domain.UIUpdate, 1)
		ch1 <- domain.DoneEvent{}
		close(ch1)
		bus.On("UIUpdates").Return((<-chan domain.UIUpdate)(ch1)).Once()

		m := NewModel(bus, theme)
		m.Update(result)
		require.NotNil(t, m.picker)

		m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		assert.True(t, m.cancelRequested)

		// While cancelled, a new snapshot should not reinitialize picker/state.
		prevPicker := m.picker
		ch2 := make(chan domain.UIUpdate, 1)
		ch2 <- domain.ModelListEvent{Models: []domain.LLMInfo{{ID: "mx", DisplayName: "X"}}, ActiveModelID: ""}
		close(ch2)
		bus.On("UIUpdates").Return((<-chan domain.UIUpdate)(ch2)).Once()

		_, cmd := m.Update(domain.ModelListEvent{Models: []domain.LLMInfo{{ID: "mx", DisplayName: "X"}}, ActiveModelID: ""})
		assert.NotNil(t, cmd)
		_ = cmd()
		assert.Equal(t, prevPicker, m.picker)
	})
}
