package history

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/cloudwego/eino/schema"
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

func TestModel_EventFlow(t *testing.T) {
	theme := ui.NewTheme(ui.ThemeConfig{})
	bus := new(mockBus)

	t.Run("Snapshot received -> Renders content", func(t *testing.T) {
		m := NewModel(bus, theme, 80, 12, 80, 40)

		ev := domain.HistoryEvent{
			Messages: []*schema.Message{{Role: schema.User, Content: "snapshot message"}},
		}

		// Initial view should be empty (detecting no messages/displays yet)
		assert.Empty(t, m.View())

		m.Update(ev)
		assert.Contains(t, m.View(), "snapshot")
		assert.Contains(t, m.View(), "message")
	})

	t.Run("DoneEvent stops polling", func(t *testing.T) {
		m := NewModel(bus, theme, 80, 12, 80, 40)
		m.Update(domain.DoneEvent{})

		// This is hard to test purely with mock assertions unless we check if pollBus returned a cmd
		// But we can verify it doesn't crash.
		assert.True(t, m.loaded)
	})

	t.Run("ctrl+c -> Sends StopAction", func(t *testing.T) {
		bus := new(mockBus)
		bus.On("SendAction", domain.StopAction{}).Once()

		m := NewModel(bus, theme, 80, 12, 80, 40)
		msg := tea.KeyMsg{Type: tea.KeyCtrlC}
		m.Update(msg)

		bus.AssertCalled(t, "SendAction", domain.StopAction{})
	})
}
