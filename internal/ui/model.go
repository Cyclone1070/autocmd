package ui

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
)

type model struct {
	spinner spinner.Model
	glamour *glamour.TermRenderer

	// State
	thinking    bool
	currentTool *toolState
	history     []string // Rendered strings of past events

	err error
}

type toolState struct {
	callID  string
	name    string
	display domain.ToolDisplay
	// Specific state for shell/diff can be stored here or inside Display structs if they were mutable (they aren't)
	// We might need to wrap them.
	// For Shell: output content accumulator
	shellOutput strings.Builder
	// Status
	status toolStatus
	err    string
}

type toolStatus int

const (
	statusRunning toolStatus = iota
	statusSuccess
	statusError
)

func newModel() *model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle

	r, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)

	return &model{
		spinner: s,
		glamour: r,
	}
}

func (m *model) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case Msg:
		return m.handleEvent(msg.Event)

	case spinner.TickMsg:
		if m.thinking || (m.currentTool != nil && m.currentTool.status == statusRunning) {
			var newSpinner spinner.Model
			newSpinner, cmd = m.spinner.Update(msg)
			m.spinner = newSpinner
			return m, cmd
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m *model) handleEvent(ev domain.Event) (tea.Model, tea.Cmd) {
	switch e := ev.(type) {
	case domain.ThinkingEvent:
		m.thinking = true
		return m, m.spinner.Tick

	case domain.TextEvent:
		m.thinking = false
		// Render markdown
		out, err := m.glamour.Render(e.Text)
		if err != nil {
			out = e.Text // Fallback
		}
		// Print persistently above the TUI
		return m, tea.Println(out)

	case domain.ToolStartEvent:
		m.thinking = false
		m.currentTool = &toolState{
			callID:  e.CallID,
			name:    e.ToolName,
			display: e.Display,
			status:  statusRunning,
		}

		// If it's a diff, we might want to pre-render it?
		// For shell, we start with empty output.
		return m, m.spinner.Tick

	case domain.ToolStreamEvent:
		if m.currentTool != nil && m.currentTool.callID == e.CallID {
			m.currentTool.shellOutput.WriteString(e.Chunk)
		}
		return m, nil

	case domain.ToolEndEvent:
		if m.currentTool != nil && m.currentTool.callID == e.CallID {
			if e.Error != "" {
				m.currentTool.status = statusError
				m.currentTool.err = e.Error
			} else {
				m.currentTool.status = statusSuccess
			}
			// Print persistently above the TUI
			output := m.viewTool(m.currentTool)
			m.currentTool = nil
			return m, tea.Println(output)
		}
		return m, nil

	case domain.DoneEvent:
		return m, tea.Quit
	}

	return m, nil
}

func (m *model) View() string {
	// Only show current active state (thinking or running tool)
	// Completed items are printed via tea.Println and persist above this
	if m.thinking {
		return fmt.Sprintf("%s Thinking...", m.spinner.View())
	}
	if m.currentTool != nil {
		return m.viewTool(m.currentTool)
	}
	return ""
}
