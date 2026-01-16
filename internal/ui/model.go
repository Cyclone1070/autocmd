package ui

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
)

type model struct {
	spinner spinner.Model
	glamour *glamour.TermRenderer
	theme   *theme
	config  *config.Config

	// State
	width         int
	thinking      bool
	activeTools   map[string]*toolState // Keyed by CallID
	toolOrder     []string              // CallIDs in order of creation (for stable rendering)
	streamingText string                // Accumulated text for live streaming
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

func newModel(cfg *config.Config) *model {
	th := newTheme(cfg.UI)
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = th.spinner

	r, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)

	return &model{
		spinner:     s,
		glamour:     r,
		theme:       th,
		config:      cfg,
		width:       cfg.UI.ChatWindowWidth,
		activeTools: make(map[string]*toolState),
	}
}

func (m *model) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m *model) Update(teaMsg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch ev := teaMsg.(type) {
	case msg:
		return m.handleEvent(ev.Event)

	case spinner.TickMsg:
		// Tick if thinking OR any tool is active
		if m.thinking || len(m.activeTools) > 0 {
			var newSpinner spinner.Model
			newSpinner, cmd = m.spinner.Update(ev)
			m.spinner = newSpinner
			return m, cmd
		}

	case tea.KeyMsg:
		switch ev.String() {
		case "ctrl+c":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = min(ev.Width, m.config.UI.ChatWindowWidth)
		// Re-initialize glamour with new width
		r, _ := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(m.width),
		)
		m.glamour = r
		return m, nil
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
		m.streamingText += e.Text
		return m, nil

	case domain.ToolStartEvent:
		var cmds []tea.Cmd
		m.thinking = false

		// Flush streaming text if any
		if m.streamingText != "" {
			out, err := m.glamour.Render(m.streamingText)
			if err != nil {
				out = m.streamingText
			}
			cmds = append(cmds, tea.Println(out))
			m.streamingText = ""
		}

		// Initialize new tool state
		ts := &toolState{
			callID:  e.CallID,
			name:    e.ToolName,
			display: e.Display,
			status:  statusRunning,
		}
		m.activeTools[e.CallID] = ts
		m.toolOrder = append(m.toolOrder, e.CallID)

		// Ensure spinner is ticking
		if len(m.activeTools) == 1 {
			cmds = append(cmds, m.spinner.Tick)
		}
		return m, tea.Batch(cmds...)

	case domain.ToolStreamEvent:
		if ts, ok := m.activeTools[e.CallID]; ok {
			ts.shellOutput.WriteString(e.Chunk)
		}
		return m, nil

	case domain.ToolEndEvent:
		if ts, ok := m.activeTools[e.CallID]; ok {
			if e.Error != "" {
				ts.status = statusError
				ts.err = e.Error
			} else {
				ts.status = statusSuccess
			}

			// Render final output
			output := m.viewTool(ts)

			// Remove from active state
			delete(m.activeTools, e.CallID)

			// Remove from order slice (careful implementation)
			newOrder := make([]string, 0, len(m.toolOrder)-1)
			for _, id := range m.toolOrder {
				if id != e.CallID {
					newOrder = append(newOrder, id)
				}
			}
			m.toolOrder = newOrder

			return m, tea.Println(output)
		}
		return m, nil

	case domain.DoneEvent:
		if m.streamingText != "" {
			out, err := m.glamour.Render(m.streamingText)
			if err != nil {
				out = m.streamingText
			}
			return m, tea.Sequence(tea.Println(out), tea.Quit)
		}
		return m, tea.Quit
	}

	return m, nil
}

func (m *model) View() string {
	// Only show current active state (streaming text, tool, or thinking)
	// Completed items are printed via tea.Println and persist above this

	// 1. Streaming text takes priority for visibility
	if m.streamingText != "" {
		out, err := m.glamour.Render(m.streamingText)
		if err != nil {
			out = m.streamingText
		}
		return out
	}

	// 2. Then active tools (stacked)
	if len(m.activeTools) > 0 {
		var toolViews []string
		for _, id := range m.toolOrder {
			if ts, ok := m.activeTools[id]; ok {
				toolViews = append(toolViews, m.viewTool(ts))
			}
		}
		return strings.Join(toolViews, "\n")
	}

	// 3. Then thinking spinner
	if m.thinking {
		return fmt.Sprintf("%s Thinking...", m.spinner.View())
	}

	return ""
}
