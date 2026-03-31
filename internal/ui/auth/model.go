package authui

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type bus interface {
	UIUpdates() <-chan domain.UIUpdate
	SendAction(domain.Action)
}

type uiState int

const (
	stateProviderSelection uiState = iota
	stateMethodSelection
	stateFieldCollection
	stateOAuthFlow
)

type oauthInfo struct {
	uri  string
	code string
}

// model is an autonomous UI component for managing authentication.
type model struct {
	bus        bus
	theme      *ui.Theme
	state      uiState
	providerID string
	method     domain.AuthMethod
	values     map[string]string
	fieldIndex int

	picker    *ui.Picker
	textInput textinput.Model
	oauth     oauthInfo
	quitting  bool
	cancelRequested bool
	err       error
}

// NewModel creates a new auth UI model.
func NewModel(b bus, theme *ui.Theme) tea.Model {
	return &model{
		bus:    b,
		theme:  theme,
		state:  stateProviderSelection,
		values: make(map[string]string),
	}
}

// Init starts the auth polling process.
func (m *model) Init() tea.Cmd {
	return m.pollBus()
}

func (m *model) pollBus() tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-m.bus.UIUpdates()
		if !ok {
			return tea.Sequence(
				tea.Printf("\n %s\n", m.theme.Error("Error: bus closed unexpectedly")),
				tea.Quit,
			)()
		}
		return ev
	}
}

// Update handles UI interactions and translates them into workflow actions.
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	if m.cancelRequested {
		switch msg.(type) {
		case domain.DoneEvent:
			// handled below
		default:
			// Keep polling until workflow terminates.
			return m, m.pollBus()
		}
	}

	switch msg := msg.(type) {
	case domain.DoneEvent:
		m.quitting = true
		if m.providerID != "" {
			return m, tea.Sequence(
				tea.Printf("\nAuthorized %s\n", m.providerID),
				tea.Quit,
			)
		}
		return m, tea.Quit

	case domain.AuthProviderListEvent:
		m.state = stateProviderSelection
		m.initializeProviderPicker(msg.Providers)
		return m, tea.Batch(m.pollBus(), m.picker.Init())

	case domain.AuthMethodEvent:
		m.state = stateMethodSelection
		m.providerID = msg.ProviderID
		m.initializeMethodPicker(msg.ProviderID, msg.Methods)
		return m, m.pollBus()

	case domain.CredentialFieldEvent:
		m.state = stateFieldCollection
		m.method = msg.Method
		m.fieldIndex = msg.FieldIndex
		m.initializeTextInput()
		return m, m.pollBus()

	case domain.OAuthDeviceFlowEvent:
		m.state = stateOAuthFlow
		m.oauth = oauthInfo{uri: msg.VerificationURI, code: msg.UserCode}
		return m, m.pollBus()

	case domain.AuthErrorEvent:
		m.err = fmt.Errorf("%s", msg.Error)
		// Don't quit! Show the error and keep polling.
		return m, m.pollBus()

	case domain.EnvVarInstructionEvent:
		m.quitting = true
		text := fmt.Sprintf("\n  %s %s\n", m.theme.Success("Instructions:"), "This provider relies on Environment Variables. Please set: " + strings.Join(msg.EnvVars, ", "))
		return m, tea.Sequence(
			tea.Printf("%s", text),
			tea.Quit,
		)

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancelRequested = true
			m.providerID = ""
			m.bus.SendAction(domain.StopAction{})
			return m, m.pollBus()
		case "q":
			if m.state != stateFieldCollection {
				m.cancelRequested = true
				m.providerID = ""
				m.bus.SendAction(domain.StopAction{})
				return m, m.pollBus()
			}
		}

		switch m.state {
		case stateProviderSelection:
			if msg.String() == "d" {
				if item, ok := m.picker.CursorItem(); ok {
					m.bus.SendAction(domain.RemoveAuthAction{ProviderID: item.ID})
					return m, nil
				}
			}
			newPicker, cmd := m.picker.Update(msg)
			m.picker = newPicker.(*ui.Picker)
			if item, ok := m.picker.Selected(); ok {
				m.bus.SendAction(domain.SelectProviderAction{ID: item.ID})
				return m, nil
			}
			return m, cmd

		case stateMethodSelection:
			newPicker, cmd := m.picker.Update(msg)
			m.picker = newPicker.(*ui.Picker)
			if item, ok := m.picker.Selected(); ok {
				m.bus.SendAction(domain.SelectAuthMethodAction{ID: item.ID})
				return m, nil
			}
			return m, cmd

		case stateFieldCollection:
			if msg.Type == tea.KeyEnter {
				val := strings.TrimSpace(m.textInput.Value())
				// Local validation
				if val == "" {
					return m, nil // Don't submit empty fields?
				}
				
				apiKeyMeth, ok := m.method.(domain.APIKeyAuthMethod)
				if !ok {
					return m, nil
				}
				field := apiKeyMeth.Fields[m.fieldIndex]
				m.values[field.ID] = val

				if m.fieldIndex+1 >= len(apiKeyMeth.Fields) {
					// All fields collected, save auth
					cred := domain.Credential{Type: apiKeyMeth.ID}
					if apiKeyMeth.ID == domain.AuthMethodAPIKey {
						cred.APIKey = m.values[domain.AuthFieldAPIKey]
					}
					m.bus.SendAction(domain.SubmitCredentialAction{Credential: cred})
					return m, nil
				}

				// Master Rule: Workflow drives next field.
				m.bus.SendAction(domain.SubmitFieldAction{Value: val})
				return m, nil
			}
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

func (m *model) initializeProviderPicker(providers []domain.ProviderSummary) {
	var items []ui.Item
	for _, p := range providers {
		detail := ""
		if p.Authorized {
			detail = fmt.Sprintf("(%s)", p.AuthMethod)
		}
		items = append(items, ui.Item{
			ID:     p.ID,
			Label:  p.ID,
			Detail: detail,
			Active: p.Authorized,
		})
	}
	m.picker = ui.NewPicker(ui.Config{
		Title: "SELECT PROVIDER",
		Items: items,
		Theme: m.theme,
		Actions: []ui.Action{
			{Key: "d", Label: "delete auth"},
		},
	})
}

func (m *model) initializeMethodPicker(providerID string, methods []domain.AuthMethod) {
	var items []ui.Item
	for _, meth := range methods {
		var id, name string
		switch v := meth.(type) {
		case domain.APIKeyAuthMethod:
			id, name = v.ID, v.Name
		case domain.OAuthMethod:
			id, name = v.ID, v.Name
		case domain.EnvVarAuthMethod:
			id, name = v.ID, v.Name
		}
		items = append(items, ui.Item{
			ID:    id,
			Label: name,
		})
	}
	m.picker = ui.NewPicker(ui.Config{
		Title: fmt.Sprintf("SELECT AUTH MODE (%s)", providerID),
		Items: items,
		Theme: m.theme,
	})
}

func (m *model) initializeTextInput() {
	apiKeyMeth, ok := m.method.(domain.APIKeyAuthMethod)
	if !ok {
		return
	}
	field := apiKeyMeth.Fields[m.fieldIndex]
	m.textInput = textinput.New()
	m.textInput.Placeholder = field.Placeholder
	if field.IsSecret {
		m.textInput.EchoMode = textinput.EchoPassword
		m.textInput.EchoCharacter = '*'
	}
	m.textInput.Focus()
}

// View determines what content to display.
func (m *model) View() string {
	if m.quitting || m.err != nil {
		if m.err != nil {
			return fmt.Sprintf("\n  %s\n\n", m.theme.Error(m.err.Error()))
		}
		return ""
	}
	switch m.state {
	case stateProviderSelection, stateMethodSelection:
		if m.picker != nil {
			return m.picker.View()
		}
	case stateFieldCollection:
		apiKeyMeth, ok := m.method.(domain.APIKeyAuthMethod)
		if !ok {
			return ""
		}
		if m.fieldIndex >= len(apiKeyMeth.Fields) {
			return "\n  Saving...\n\n"
		}
		field := apiKeyMeth.Fields[m.fieldIndex]
		return fmt.Sprintf("\n  %s\n\n  %s\n\n", field.Label, m.textInput.View())
	case stateOAuthFlow:
		var s strings.Builder
		s.WriteString(fmt.Sprintf("\n  %s\n\n", m.theme.Muted("OAuth Device Authorization:")))
		s.WriteString(fmt.Sprintf("  1. Visit: %s\n", m.theme.Primary(m.oauth.uri)))
		s.WriteString(fmt.Sprintf("  2. Enter code: %s\n", m.theme.Success(m.oauth.code)))
		s.WriteString(fmt.Sprintf("\n  %s\n", m.theme.Muted("Waiting for authorization...")))
		return s.String()
	}
	return ""
}
