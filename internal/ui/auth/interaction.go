package authui

import (
	"context"
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/auth"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/llm"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/Cyclone1070/iav/internal/ui/picker"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type uiState int

const (
	stateProviderSelection uiState = iota
	stateMethodSelection
	stateFieldCollection
)

func Run(registry *llm.Registry, authMgr *auth.Manager, appState *state.State) error {
	m := NewModel(registry, authMgr, appState)
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return err
	}
	if authModel, ok := finalModel.(*Model); ok && authModel.err != nil {
		return authModel.err
	}
	return nil
}

type Model struct {
	registry *llm.Registry
	authMgr  *auth.Manager
	appState *state.State

	state    uiState
	provider domain.Provider
	method   domain.AuthMethod
	values   map[string]string

	picker     *picker.Picker
	textInput  textinput.Model
	fieldIndex int

	quitting bool
	err      error
}

func NewModel(registry *llm.Registry, authMgr *auth.Manager, appState *state.State) *Model {
	m := &Model{
		registry: registry,
		authMgr:  authMgr,
		appState: appState,
		state:    stateProviderSelection,
		values:   make(map[string]string),
	}
	m.initProviderPicker()
	return m
}

func (m *Model) initProviderPicker() {
	infos, _ := m.registry.ListProviders(context.Background())
	var items []picker.Item
	for _, info := range infos {
		label := info.ID
		active := false
		detail := ""

		if info.Credential != nil {
			active = true
			detail = "(Authorized)"
		}

		items = append(items, picker.Item{
			ID:     info.ID,
			Label:  label,
			Active: active,
			Detail: detail,
		})
	}
	m.picker = picker.NewPicker(picker.Config{
		Title: "SELECT PROVIDER",
		Items: items,
		Actions: []picker.Action{
			{Key: "d", Label: "delete auth", Fn: func(item picker.Item) tea.Cmd { return nil }},
		},
	})
}

func (m *Model) initMethodPicker() {
	methods := m.provider.SupportedAuthMethods()
	var items []picker.Item
	for _, meth := range methods {
		items = append(items, picker.Item{
			ID:    meth.ID,
			Label: meth.Label,
		})
	}
	m.picker = picker.NewPicker(picker.Config{
		Title: fmt.Sprintf("SELECT AUTH MODE (%s)", m.provider.ID()),
		Items: items,
	})
}

func (m *Model) initTextInput() {
	m.textInput = textinput.New()
	field := m.method.Fields[m.fieldIndex]
	m.textInput.Placeholder = field.Placeholder
	if field.IsSecret {
		m.textInput.EchoMode = textinput.EchoPassword
		m.textInput.EchoCharacter = '*'
	}
	m.textInput.Focus()
}

func (m *Model) Init() tea.Cmd {
	return nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.quitting = true
			return m, tea.Quit
		}
	}

	// Always clear validation error on any keypress
	m.err = nil 

	var cmd tea.Cmd
	switch m.state {
	case stateProviderSelection:
		// Handle custom action 'd' for deletion
		if msg, ok := msg.(tea.KeyMsg); ok && msg.String() == "d" {
			if item, ok := m.picker.CursorItem(); ok {
				m.deleteAuth(item.ID)
				m.initProviderPicker()
			}
			return m, nil
		}

		newPicker, pCmd := m.picker.Update(msg)
		m.picker = newPicker.(*picker.Picker)
		
		if sel, ok := m.picker.Selected(); ok {
			m.provider, _ = m.registry.GetProvider(sel.ID)
			m.state = stateMethodSelection
			m.initMethodPicker()
		} else {
			cmd = pCmd
		}

	case stateMethodSelection:
		newPicker, pCmd := m.picker.Update(msg)
		m.picker = newPicker.(*picker.Picker)
		
		if sel, ok := m.picker.Selected(); ok {
			for _, meth := range m.provider.SupportedAuthMethods() {
				if meth.ID == sel.ID {
					m.method = meth
					break
				}
			}
			m.state = stateFieldCollection
			m.fieldIndex = 0
			m.initTextInput()
		} else {
			cmd = pCmd
		}

	case stateFieldCollection:
		var tCmd tea.Cmd
		m.textInput, tCmd = m.textInput.Update(msg)
		cmd = tCmd

		if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEnter {
			val := m.textInput.Value()
			if val == "" {
				m.err = fmt.Errorf("field cannot be empty")
				m.quitting = true
				return m, tea.Quit
			}
			
			field := m.method.Fields[m.fieldIndex]
			m.values[field.ID] = val
			
			m.fieldIndex++
			if m.fieldIndex >= len(m.method.Fields) {
				// ALL DONE
				if err := m.save(); err != nil {
					m.err = err
					m.quitting = true
					return m, tea.Quit
				}
				m.quitting = true
				return m, tea.Quit
			}
			m.initTextInput()
		}
	}

	return m, cmd
}

func (m *Model) save() error {
	cred := domain.Credential{
		Type: m.method.ID,
	}
	if m.method.ID == domain.AuthMethodAPIKey {
		cred.APIKey = m.values[domain.AuthFieldAPIKey]
	}
	// Future: handle Vertex AI etc.
	return m.authMgr.Set(m.provider.ID(), cred)
}

func (m *Model) deleteAuth(providerID string) {
	_ = m.authMgr.Remove(providerID)
	// Clear current model if it belongs to this provider
	if strings.HasPrefix(m.appState.Model, providerID+"/") {
		m.appState.Model = ""
		_ = state.Save(m.appState)
	}
}

func (m *Model) View() string {
	if m.err != nil || m.quitting {
		return ""
	}

	switch m.state {
	case stateProviderSelection, stateMethodSelection:
		return m.picker.View()
	case stateFieldCollection:
		if m.fieldIndex >= len(m.method.Fields) {
			return ""
		}
		field := m.method.Fields[m.fieldIndex]
		return fmt.Sprintf("\n  %s\n\n  %s\n\n", field.Label, m.textInput.View())
	default:
		return ""
	}
}
