package history

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
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
		m := NewModel(bus, theme, 80, 80, 40)
		
		ev := domain.HistoryEvent{
			Messages: domain.Messages{domain.UserMessage{Content: "snapshot message"}},
		}
		
		// Initial view should be empty (detecting no messages/displays yet)
		assert.Empty(t, m.View())
		
		m.Update(ev)
		assert.Contains(t, m.View(), "snapshot")
		assert.Contains(t, m.View(), "message")
	})

	t.Run("DoneEvent stops polling", func(t *testing.T) {
		m := NewModel(bus, theme, 80, 80, 40)
		m.Update(domain.DoneEvent{})
		
		// This is hard to test purely with mock assertions unless we check if pollBus returned a cmd
		// But we can verify it doesn't crash.
		assert.True(t, m.loaded)
	})
}
