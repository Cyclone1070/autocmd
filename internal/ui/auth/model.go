// Package authui provides the UI components for the authentication workflow.
package authui

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/autocmd/internal/domain"
	"github.com/Cyclone1070/autocmd/internal/ui"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

const pickerActionLabelBack = "back"

type oauthInfo struct {
	uri  string
	code string
}

// model is an autonomous UI component for managing authentication.
type model struct {
	method          domain.AuthMethod
	err             error
	bus             bus
	picker          *ui.Picker
	values          map[string]string
	theme           *ui.Theme
	oauth           oauthInfo
	providerID      string
	methods         []domain.AuthMethod
	providers       []domain.ProviderSummary
	textInput       textinput.Model
	fieldIndex      int
	state           uiState
	quitting        bool
	cancelRequested bool
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
	if m.cancelRequested {
		if _, ok := msg.(domain.DoneEvent); !ok {
			return m, m.pollBus()
		}
	}

	switch msg := msg.(type) {
	case domain.UIUpdate:
		return m.handleDomainEvent(msg)
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	default:
		return m, nil
	}
}

func (m *model) handleDomainEvent(msg domain.UIUpdate) (tea.Model, tea.Cmd) {
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
		m.providers = msg.Providers
		m.initializeProviderPicker(msg.Providers)
		return m, tea.Batch(m.pollBus(), m.picker.Init())

	case domain.AuthMethodEvent:
		m.state = stateMethodSelection
		m.providerID = msg.ProviderID
		m.methods = msg.Methods
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
		return m, m.pollBus()

	case domain.EnvVarInstructionEvent:
		m.quitting = true
		text := fmt.Sprintf("\n  %s %s\n", m.theme.Success("Instructions:"), "This provider relies on Environment Variables. Please set: "+strings.Join(msg.EnvVars, ", "))
		return m, tea.Sequence(
			tea.Printf("%s", text),
			tea.Quit,
		)
	}
	return m, nil
}

func (m *model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.cancelRequested = true
		m.providerID = ""
		m.bus.SendAction(domain.StopAction{})
		return m, m.pollBus()
	case "esc":
		if m.handleBackKey(msg) {
			return m, nil
		}
		return m, nil
	case "q":
		if m.state != stateFieldCollection {
			m.cancelRequested = true
			m.providerID = ""
			m.bus.SendAction(domain.StopAction{})
			return m, m.pollBus()
		}
	}

	if m.handleBackKey(msg) {
		return m, nil
	}

	switch m.state {
	case stateProviderSelection:
		return m.handleProviderSelectionKey(msg)
	case stateMethodSelection:
		return m.handleMethodSelectionKey(msg)
	case stateFieldCollection:
		return m.handleFieldCollectionKey(msg)
	}

	return m, nil
}

func (m *model) handleProviderSelectionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
}

func (m *model) handleMethodSelectionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	newPicker, cmd := m.picker.Update(msg)
	m.picker = newPicker.(*ui.Picker)
	if item, ok := m.picker.Selected(); ok {
		m.bus.SendAction(domain.SelectAuthMethodAction{ID: item.ID})
		return m, nil
	}
	return m, cmd
}

func (m *model) handleFieldCollectionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEnter {
		val := strings.TrimSpace(m.textInput.Value())
		if val == "" {
			return m, nil
		}

		apiKeyMeth, ok := m.method.(domain.APIKeyAuthMethod)
		if !ok {
			return m, nil
		}
		field := apiKeyMeth.Fields[m.fieldIndex]
		m.values[field.ID] = val

		if m.fieldIndex+1 >= len(apiKeyMeth.Fields) {
			cred := domain.Credential{Type: apiKeyMeth.ID}
			if apiKeyMeth.ID == domain.AuthMethodAPIKey {
				cred.APIKey = m.values[domain.AuthFieldAPIKey]
			}
			m.bus.SendAction(domain.SubmitCredentialAction{Credential: cred})
			return m, nil
		}

		m.bus.SendAction(domain.SubmitFieldAction{Value: val})
		return m, nil
	}
	if msg.Type == tea.KeySpace {
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m *model) initializeProviderPicker(providers []domain.ProviderSummary) {
	items := make([]ui.Item, 0, len(providers))
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
	items := make([]ui.Item, 0, len(methods))
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
		Actions: []ui.Action{
			{Key: "Backspace", Label: pickerActionLabelBack},
		},
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

func (m *model) handleBackKey(msg tea.KeyMsg) bool {
	switch m.state {
	case stateMethodSelection:
		if !isMethodBackKey(msg) && msg.Type != tea.KeyEsc {
			return false
		}
		m.state = stateProviderSelection
		m.initializeProviderPicker(m.providers)
		return true
	case stateFieldCollection:
		if msg.Type != tea.KeyEsc {
			return false
		}
		m.state = stateMethodSelection
		m.initializeMethodPicker(m.providerID, m.methods)
		return true
	case stateOAuthFlow:
		if !isOAuthBackKey(msg) {
			return false
		}
		m.state = stateMethodSelection
		m.initializeMethodPicker(m.providerID, m.methods)
		return true
	default:
		return false
	}
}

func isMethodBackKey(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyBackspace, tea.KeyLeft:
		return true
	}
	return msg.String() == "h"
}

func isOAuthBackKey(msg tea.KeyMsg) bool {
	if msg.Type == tea.KeyEsc {
		return true
	}
	return isMethodBackKey(msg)
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
		var s strings.Builder
		fmt.Fprintf(&s, "  %s\n\n", m.renderStageTitle(m.fieldTitle(field.Label, m.fieldIndex, len(apiKeyMeth.Fields))))
		fmt.Fprintf(&s, "  %s\n\n", m.renderHelpRow([]helpKey{
			{key: "Enter", label: "save"},
			{key: "Esc", label: pickerActionLabelBack},
			{key: "Ctrl+c", label: "quit"},
		}))
		fmt.Fprintf(&s, "  %s\n\n", m.textInput.View())
		return s.String()
	case stateOAuthFlow:
		var s strings.Builder
		fmt.Fprintf(&s, "  %s\n\n", m.renderStageTitle("OAUTH DEVICE AUTHORIZATION"))
		fmt.Fprintf(&s, "  %s\n\n", m.renderHelpRow([]helpKey{
			{key: "Backspace", label: pickerActionLabelBack},
			{key: "q", label: "quit"},
		}))
		fmt.Fprintf(&s, "  1. Visit: %s\n", m.theme.Primary(m.oauth.uri))
		fmt.Fprintf(&s, "  2. Enter code: %s\n", m.theme.Success(m.oauth.code))
		fmt.Fprintf(&s, "\n  %s\n", m.theme.Muted("Waiting for authorization..."))
		return s.String()
	}
	return ""
}

type helpKey struct {
	key   string
	label string
}

func (m *model) renderHelpRow(keys []helpKey) string {
	parts := make([]string, 0, len(keys))
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(m.theme.PrimaryColor())
	labelStyle := lipgloss.NewStyle().Foreground(m.theme.MutedColor())
	for _, item := range keys {
		parts = append(parts, fmt.Sprintf("%s %s", keyStyle.Render(item.key), labelStyle.Render(item.label)))
	}
	return strings.Join(parts, "   ")
}

func (m *model) renderStageTitle(title string) string {
	return lipgloss.NewStyle().Bold(true).Foreground(m.theme.PrimaryColor()).Render(title)
}

func (m *model) fieldTitle(label string, idx, total int) string {
	if total <= 1 {
		return label
	}
	return fmt.Sprintf("%s (%d/%d)", label, idx+1, total)
}
