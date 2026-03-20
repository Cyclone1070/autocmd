package info

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
	return args.Get(0).(chan domain.UIUpdate)
}

func (m *mockBus) SendAction(action domain.Action) {
	m.Called(action)
}

func TestModel(t *testing.T) {
	t.Run("Receives InfoEvent", func(t *testing.T) {
		bus := new(mockBus)
		eventChan := make(chan domain.UIUpdate, 1)
		bus.On("UIUpdates").Return(eventChan)

		theme := ui.NewTheme(ui.ThemeConfig{})
		m := NewModel(bus, theme)
		
		info := domain.InfoEvent{Model: "gpt-4"}
		eventChan <- info

		// BubbleTea tick manually
		newModel, cmd := m.Update(info)
		m = newModel.(*Model)

		// Check that it returns a command that eventually calls Printf (wrapped in tea.Sequence)
		assert.NotNil(t, cmd)
	})

	t.Run("Quits on DoneEvent", func(t *testing.T) {
		bus := new(mockBus)
		eventChan := make(chan domain.UIUpdate, 1)
		bus.On("UIUpdates").Return(eventChan)

		theme := ui.NewTheme(ui.ThemeConfig{})
		m := NewModel(bus, theme)
		
		_, cmd := m.Update(domain.DoneEvent{})

		assert.Equal(t, tea.Quit(), cmd())
	})

	t.Run("Handles Bus Error", func(t *testing.T) {
		bus := new(mockBus)
		// No event sent, channel closed
		eventChan := make(chan domain.UIUpdate)
		close(eventChan)
		bus.On("UIUpdates").Return(eventChan)

		theme := ui.NewTheme(ui.ThemeConfig{})
		m := NewModel(bus, theme)
		
		cmd := m.pollBus()
		// Should return a command that eventually calls Printf and Quit
		assert.NotNil(t, cmd)
	})
}

func TestRenderInfo(t *testing.T) {
	t.Run("Full Success Scenario", func(t *testing.T) {
		data := &domain.InfoEvent{
			Model:          "google/gemini-pro",
			SessionDisplay: "Test Session",
			SessionTokens:  100,
			ContextWindow:  128000,
			Authorized:     []string{"google (env)"},
		}

		output := renderInfo(data)
		assert.Contains(t, output, "Model:")
		assert.Contains(t, output, "google/gemini-pro")
		assert.Contains(t, output, "Current Session:")
		assert.Contains(t, output, "Test Session")
		assert.Contains(t, output, "Session Usage:")
		assert.Contains(t, output, "100 tokens")
		assert.Contains(t, output, "0.1% of 128000 context")
		assert.Contains(t, output, "Authorized Providers:")
		assert.Contains(t, output, "google (env)")
	})

	t.Run("Minimal Scenario", func(t *testing.T) {
		data := &domain.InfoEvent{
			SessionDisplay: "none",
		}

		output := renderInfo(data)
		assert.Contains(t, output, "Current Session:")
		assert.Contains(t, output, "none")
		assert.NotContains(t, output, "Model:")
		assert.NotContains(t, output, "Authorized Providers:")
	})
}
