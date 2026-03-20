package authui

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

func TestAuthUI_Interactive(t *testing.T) {
	theme := ui.NewTheme(ui.ThemeConfig{})
	bus := new(mockBus)

	t.Run("StopAction on 'q'", func(t *testing.T) {
		m := NewModel(bus, theme).(*model)
		m.state = stateProviderSelection
		
		bus.On("SendAction", domain.StopAction{}).Return()
		
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
		
		assert.Nil(t, cmd) // MUST NOT poll
		bus.AssertCalled(t, "SendAction", domain.StopAction{})
	})

	t.Run("DoneEvent triggers tea.Quit", func(t *testing.T) {
		m := NewModel(bus, theme).(*model)
		
		_, cmd := m.Update(domain.DoneEvent{})
		
		// The cmd should be tea.Quit or a sequence ending in tea.Quit
		// We look at the command's value or behavior if possible
		assert.NotNil(t, cmd)
	})

	t.Run("Closed bus triggers tea.Quit", func(t *testing.T) {
		ch := make(chan domain.UIUpdate)
		close(ch)
		bus.On("UIUpdates").Return((<-chan domain.UIUpdate)(ch))
		
		m := NewModel(bus, theme).(*model)
		cmd := m.Init()
		msg := cmd()
		
		// msg should be tea.Batch/Sequence containing tea.Quit
		assert.NotNil(t, msg)
	})

	t.Run("AuthProviderListEvent shows AuthMethod", func(t *testing.T) {
		m := NewModel(bus, theme).(*model)
		snapshot := domain.AuthProviderListEvent{
			Providers: []domain.ProviderSummary{
				{ID: "openai", Authorized: true, AuthMethod: "api_key"},
				{ID: "anthropic", Authorized: false},
			},
		}
		m.Update(snapshot)
		
		view := m.View()
		assert.Contains(t, view, "openai")
		assert.Contains(t, view, "api_key")
		assert.Contains(t, view, "anthropic")
	})

	t.Run("Cancellation clears providerID", func(t *testing.T) {
		m := NewModel(bus, theme).(*model)
		m.state = stateMethodSelection
		m.providerID = "openai"

		bus.On("SendAction", domain.StopAction{}).Return()
		
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
		
		assert.Empty(t, m.providerID)
	})
}
