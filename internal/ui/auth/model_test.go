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
		
		ch := make(chan domain.UIUpdate, 1)
		ch <- domain.DoneEvent{}
		close(ch)
		bus.On("UIUpdates").Return((<-chan domain.UIUpdate)(ch))
		bus.On("SendAction", domain.StopAction{}).Return()
		
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
		
		assert.NotNil(t, cmd, "cancel should keep polling for DoneEvent (not quit immediately)")
		msg := cmd()
		_, ok := msg.(domain.DoneEvent)
		assert.True(t, ok, "cancel should schedule pollBus (next msg should be DoneEvent)")
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
		bus := new(mockBus)
		m := NewModel(bus, theme).(*model)
		m.state = stateMethodSelection
		m.providerID = "openai"

		ch := make(chan domain.UIUpdate, 1)
		ch <- domain.DoneEvent{}
		close(ch)
		bus.On("UIUpdates").Return((<-chan domain.UIUpdate)(ch))
		bus.On("SendAction", domain.StopAction{}).Return()
		
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
		
		assert.Empty(t, m.providerID)
		assert.NotNil(t, cmd, "cancel should keep polling for DoneEvent")
		msg := cmd()
		_, ok := msg.(domain.DoneEvent)
		assert.True(t, ok, "cancel should schedule pollBus (next msg should be DoneEvent)")
	})

	t.Run("CancelRequested ignores subsequent bus events until DoneEvent", func(t *testing.T) {
		bus := new(mockBus)
		m := NewModel(bus, theme).(*model)
		m.state = stateProviderSelection

		// Cancel first (will schedule a poll).
		ch1 := make(chan domain.UIUpdate, 1)
		ch1 <- domain.DoneEvent{}
		close(ch1)
		bus.On("UIUpdates").Return((<-chan domain.UIUpdate)(ch1)).Once()
		bus.On("SendAction", domain.StopAction{}).Return().Once()

		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
		assert.True(t, m.cancelRequested)

		// While cancelled, receiving new UI updates should be ignored (but keep polling).
		ch2 := make(chan domain.UIUpdate, 1)
		ch2 <- domain.AuthProviderListEvent{Providers: []domain.ProviderSummary{{ID: "github"}}}
		close(ch2)
		bus.On("UIUpdates").Return((<-chan domain.UIUpdate)(ch2)).Once()

		prevState := m.state
		prevPicker := m.picker
		_, cmd := m.Update(domain.AuthProviderListEvent{Providers: []domain.ProviderSummary{{ID: "github"}}})
		assert.NotNil(t, cmd)
		_ = cmd() // drain the pollBus command
		assert.Equal(t, prevState, m.state, "cancelRequested must prevent state changes from new events")
		assert.Equal(t, prevPicker, m.picker, "cancelRequested must prevent picker reinitialization")
	})
	t.Run("OAuthDeviceFlowEvent shows code", func(t *testing.T) {
		m := NewModel(bus, theme).(*model)
		
		event := domain.OAuthDeviceFlowEvent{
			VerificationURI: "https://github.com/login/device",
			UserCode:        "ABCD-1234",
		}
		
		m.Update(event)
		
		view := m.View()
		assert.Contains(t, view, "https://github.com/login/device")
		assert.Contains(t, view, "ABCD-1234")
		assert.Contains(t, view, "Backspace")
		assert.Contains(t, view, "back")
		assert.Contains(t, view, "q")
		assert.NotContains(t, view, "esc")
		assert.NotContains(t, view, "←")
	})

	t.Run("Field collection: space is text, not select shortcut", func(t *testing.T) {
		bus := new(mockBus)
		m := NewModel(bus, theme).(*model)
		m.Update(domain.CredentialFieldEvent{
			Method: domain.APIKeyAuthMethod{
				ID: domain.AuthMethodAPIKey,
				Fields: []domain.AuthField{
					{ID: domain.AuthFieldAPIKey, Label: "API Key", Placeholder: "Enter key", IsSecret: false},
				},
			},
			FieldIndex: 0,
		})

		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ab")})
		m.Update(tea.KeyMsg{Type: tea.KeySpace})
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("cd")})

		assert.Equal(t, "ab cd", m.textInput.Value(), "space should be treated as text input in auth field mode")
	})

	t.Run("Field collection shows instruction row", func(t *testing.T) {
		bus := new(mockBus)
		m := NewModel(bus, theme).(*model)
		m.Update(domain.CredentialFieldEvent{
			Method: domain.APIKeyAuthMethod{
				ID: domain.AuthMethodAPIKey,
				Fields: []domain.AuthField{
					{ID: domain.AuthFieldAPIKey, Label: "API Key", Placeholder: "Enter key", IsSecret: false},
				},
			},
			FieldIndex: 0,
		})

		view := m.View()
		assert.Contains(t, view, "Enter")
		assert.Contains(t, view, "save")
		assert.Contains(t, view, "Esc")
		assert.Contains(t, view, "back")
		assert.Contains(t, view, "Ctrl+c")
	})

	t.Run("Field collection title uses current field label", func(t *testing.T) {
		bus := new(mockBus)
		m := NewModel(bus, theme).(*model)
		m.Update(domain.CredentialFieldEvent{
			Method: domain.APIKeyAuthMethod{
				ID: domain.AuthMethodAPIKey,
				Fields: []domain.AuthField{
					{ID: "org", Label: "Organization", Placeholder: "Enter org", IsSecret: false},
					{ID: "key", Label: "API Key", Placeholder: "Enter key", IsSecret: true},
				},
			},
			FieldIndex: 1,
		})

		view := m.View()
		assert.Contains(t, view, "API Key")
		assert.Contains(t, view, "(2/2)")
		assert.NotContains(t, view, "API KEY")
	})

	t.Run("Method selection back keys return to provider selection", func(t *testing.T) {
		bus := new(mockBus)
		m := NewModel(bus, theme).(*model)
		providers := []domain.ProviderSummary{{ID: "openai"}}
		methods := []domain.AuthMethod{
			domain.APIKeyAuthMethod{ID: "api_key", Name: "API Key", Fields: []domain.AuthField{{ID: "key"}}},
		}

		m.Update(domain.AuthProviderListEvent{Providers: providers})
		m.Update(domain.AuthMethodEvent{ProviderID: "openai", Methods: methods})
		assert.Equal(t, stateMethodSelection, m.state)

		backKeys := []tea.KeyMsg{
			{Type: tea.KeyBackspace},
			{Type: tea.KeyRunes, Runes: []rune("h")},
			{Type: tea.KeyLeft},
		}
		for _, key := range backKeys {
			m.Update(domain.AuthMethodEvent{ProviderID: "openai", Methods: methods})
			_, _ = m.Update(key)
			assert.Equal(t, stateProviderSelection, m.state)
			assert.Contains(t, m.View(), "SELECT PROVIDER")
		}
	})

	t.Run("Method selection shows Backspace hint", func(t *testing.T) {
		bus := new(mockBus)
		m := NewModel(bus, theme).(*model)
		providers := []domain.ProviderSummary{{ID: "openai"}}
		methods := []domain.AuthMethod{
			domain.APIKeyAuthMethod{ID: "api_key", Name: "API Key", Fields: []domain.AuthField{{ID: "key"}}},
		}

		m.Update(domain.AuthProviderListEvent{Providers: providers})
		m.Update(domain.AuthMethodEvent{ProviderID: "openai", Methods: methods})

		view := m.View()
		assert.Contains(t, view, "Backspace")
		assert.Contains(t, view, "back")
	})

	t.Run("Field collection back keys stay in field collection", func(t *testing.T) {
		bus := new(mockBus)
		m := NewModel(bus, theme).(*model)
		providers := []domain.ProviderSummary{{ID: "openai"}}
		methods := []domain.AuthMethod{
			domain.APIKeyAuthMethod{
				ID: "api_key", Name: "API Key",
				Fields: []domain.AuthField{{ID: "key", Label: "API Key", Placeholder: "Enter key"}},
			},
		}

		m.Update(domain.AuthProviderListEvent{Providers: providers})
		m.Update(domain.AuthMethodEvent{ProviderID: "openai", Methods: methods})
		m.Update(domain.CredentialFieldEvent{Method: methods[0], FieldIndex: 0})
		assert.Equal(t, stateFieldCollection, m.state)

		backKeys := []tea.KeyMsg{
			{Type: tea.KeyBackspace},
			{Type: tea.KeyRunes, Runes: []rune("h")},
			{Type: tea.KeyLeft},
		}
		for _, key := range backKeys {
			m.Update(domain.CredentialFieldEvent{Method: methods[0], FieldIndex: 0})
			m.textInput.SetValue("abc")
			_, _ = m.Update(key)
			assert.Equal(t, stateFieldCollection, m.state)
			assert.Contains(t, m.View(), "API Key")
		}
	})

	t.Run("Field collection esc returns to method selection", func(t *testing.T) {
		bus := new(mockBus)
		m := NewModel(bus, theme).(*model)
		providers := []domain.ProviderSummary{{ID: "openai"}}
		methods := []domain.AuthMethod{
			domain.APIKeyAuthMethod{
				ID: "api_key", Name: "API Key",
				Fields: []domain.AuthField{{ID: "key", Label: "API Key", Placeholder: "Enter key"}},
			},
		}

		m.Update(domain.AuthProviderListEvent{Providers: providers})
		m.Update(domain.AuthMethodEvent{ProviderID: "openai", Methods: methods})
		m.Update(domain.CredentialFieldEvent{Method: methods[0], FieldIndex: 0})

		_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		assert.Equal(t, stateMethodSelection, m.state)
		assert.Contains(t, m.View(), "SELECT AUTH MODE (openai)")
	})

	t.Run("OAuth flow back keys return to method selection", func(t *testing.T) {
		bus := new(mockBus)
		m := NewModel(bus, theme).(*model)
		providers := []domain.ProviderSummary{{ID: "github"}}
		methods := []domain.AuthMethod{
			domain.OAuthMethod{ID: "github_oauth", Name: "GitHub"},
		}
		m.Update(domain.AuthProviderListEvent{Providers: providers})
		m.Update(domain.AuthMethodEvent{ProviderID: "github", Methods: methods})
		m.Update(domain.OAuthDeviceFlowEvent{
			VerificationURI: "https://github.com/login/device",
			UserCode:        "ABCD-1234",
		})

		backKeys := []tea.KeyMsg{
			{Type: tea.KeyBackspace},
			{Type: tea.KeyEsc},
		}
		for _, key := range backKeys {
			m.Update(domain.OAuthDeviceFlowEvent{
				VerificationURI: "https://github.com/login/device",
				UserCode:        "ABCD-1234",
			})
			_, _ = m.Update(key)
			assert.Equal(t, stateMethodSelection, m.state)
			assert.Contains(t, m.View(), "SELECT AUTH MODE (github)")
		}
	})

	t.Run("Esc in method selection goes back (stealth)", func(t *testing.T) {
		bus := new(mockBus)
		m := NewModel(bus, theme).(*model)
		providers := []domain.ProviderSummary{{ID: "openai"}}
		methods := []domain.AuthMethod{
			domain.APIKeyAuthMethod{ID: "api_key", Name: "API Key", Fields: []domain.AuthField{{ID: "key"}}},
		}
		m.Update(domain.AuthProviderListEvent{Providers: providers})
		m.Update(domain.AuthMethodEvent{ProviderID: "openai", Methods: methods})

		_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		assert.Equal(t, stateProviderSelection, m.state)
		assert.Contains(t, m.View(), "SELECT PROVIDER")
	})
}
