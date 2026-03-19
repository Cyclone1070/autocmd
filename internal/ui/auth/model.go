package authui

import (
	"context"
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Workflow defines the operations needed for authentication.
type Workflow interface {
	Gather(ctx context.Context) (*domain.AuthProviderSnapshot, error)
	SetAuth(ctx context.Context, providerID string, cred domain.Credential) error
	RemoveAuth(ctx context.Context, providerID string) error
	GetProvider(id string) (domain.Provider, bool)
}

type prepareResultMsg struct {
	data *domain.AuthProviderSnapshot
	err  error
}

type mutationResultMsg struct {
	err     error
	refresh bool
}

type uiState int

const (
	stateProviderSelection uiState = iota
	stateMethodSelection
	stateFieldCollection
)

// Model is an autonomous UI component for managing authentication.
type Model struct {
	wf         Workflow
	state      uiState
	provider   domain.Provider
	method     domain.AuthMethod
	values     map[string]string
	fieldIndex int

	picker     *ui.Picker
	textInput  textinput.Model
	quitting   bool
	err        error
}

// NewModel creates a new auth UI model.
func NewModel(wf Workflow) *Model {
	return &Model{
		wf:     wf,
		state:  stateProviderSelection,
		values: make(map[string]string),
	}
}

// Init starts the auth gathering process.
func (m *Model) Init() tea.Cmd {
	return func() tea.Msg {
		res, err := m.wf.Gather(context.Background())
		return prepareResultMsg{data: res, err: err}
	}
}

// Update handles UI interactions and translates them into workflow calls.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case prepareResultMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, tea.Quit
		}
		m.initializeProviderPicker(msg.data)
		return m, m.picker.Init()

	case mutationResultMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, tea.Quit
		}
		if !msg.refresh {
			m.quitting = true
			if m.provider != nil {
				return m, tea.Sequence(
					tea.Printf("\nAuthorized %s\n", m.provider.ID()),
					tea.Quit,
				)
			}
			return m, tea.Quit
		}
		return m, m.Init()

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		case "q":
			if m.state != stateFieldCollection {
				m.quitting = true
				return m, tea.Quit
			}
		}

		switch m.state {
		case stateProviderSelection:
			if msg.String() == "d" {
				if item, ok := m.picker.CursorItem(); ok {
					return m, func() tea.Msg {
						err := m.wf.RemoveAuth(context.Background(), item.ID)
						return mutationResultMsg{err: err, refresh: true}
					}
				}
			}
			newPicker, cmd := m.picker.Update(msg)
			m.picker = newPicker.(*ui.Picker)
			if item, ok := m.picker.Selected(); ok {
				p, ok := m.wf.GetProvider(item.ID)
				if !ok {
					m.err = fmt.Errorf("provider not found: %s", item.ID)
					return m, tea.Quit
				}
				m.provider = p
				m.state = stateMethodSelection
				m.initializeMethodPicker()
				return m, nil
			}
			return m, cmd

		case stateMethodSelection:
			newPicker, cmd := m.picker.Update(msg)
			m.picker = newPicker.(*ui.Picker)
			if item, ok := m.picker.Selected(); ok {
				for _, meth := range m.provider.SupportedAuthMethods() {
					if meth.ID == item.ID {
						m.method = meth
						break
					}
				}
				m.state = stateFieldCollection
				m.fieldIndex = 0
				m.initializeTextInput()
				return m, nil
			}
			return m, cmd

		case stateFieldCollection:
			if msg.Type == tea.KeyEnter {
				val := strings.TrimSpace(m.textInput.Value())
				if val == "" {
					m.err = fmt.Errorf("field cannot be empty")
					m.quitting = true
					return m, tea.Quit
				}
				field := m.method.Fields[m.fieldIndex]
				m.values[field.ID] = val
				m.fieldIndex++

				if m.fieldIndex >= len(m.method.Fields) {
					// All fields collected, save auth
					cred := domain.Credential{Type: m.method.ID}
					if m.method.ID == domain.AuthMethodAPIKey {
						cred.APIKey = m.values[domain.AuthFieldAPIKey]
					}
					return m, func() tea.Msg {
						err := m.wf.SetAuth(context.Background(), m.provider.ID(), cred)
						if err != nil {
							return mutationResultMsg{err: err}
						}
						return mutationResultMsg{refresh: false}
					}
				}
				m.initializeTextInput()
				return m, nil
			}
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

func (m *Model) initializeProviderPicker(data *domain.AuthProviderSnapshot) {
	var items []ui.Item
	for _, p := range data.Providers {
		detail := ""
		if p.Authorized {
			detail = "(Authorized)"
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
		Actions: []ui.Action{
			{Key: "d", Label: "delete auth"},
		},
	})
}

func (m *Model) initializeMethodPicker() {
	var items []ui.Item
	for _, meth := range m.provider.SupportedAuthMethods() {
		items = append(items, ui.Item{
			ID:    meth.ID,
			Label: meth.Label,
		})
	}
	m.picker = ui.NewPicker(ui.Config{
		Title: fmt.Sprintf("SELECT AUTH MODE (%s)", m.provider.ID()),
		Items: items,
	})
}

func (m *Model) initializeTextInput() {
	field := m.method.Fields[m.fieldIndex]
	m.textInput = textinput.New()
	m.textInput.Placeholder = field.Placeholder
	if field.IsSecret {
		m.textInput.EchoMode = textinput.EchoPassword
		m.textInput.EchoCharacter = '*'
	}
	m.textInput.Focus()
}

// View determines what content to display.
func (m *Model) View() string {
	if m.quitting || m.err != nil {
		return ""
	}
	switch m.state {
	case stateProviderSelection, stateMethodSelection:
		if m.picker != nil {
			return m.picker.View()
		}
	case stateFieldCollection:
		if m.fieldIndex >= len(m.method.Fields) {
			return "\n  Saving...\n\n"
		}
		field := m.method.Fields[m.fieldIndex]
		return fmt.Sprintf("\n  %s\n\n  %s\n\n", field.Label, m.textInput.View())
	}
	return ""
}

// Err returns any error encountered.
func (m *Model) Err() error {
	return m.err
}
