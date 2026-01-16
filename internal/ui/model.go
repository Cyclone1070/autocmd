package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"golang.org/x/term"
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
	display domain.ToolDisplay
	status  toolStatus
	err     string

	// For shell tools, we keep the output buffer
	shellOutput strings.Builder
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
	s.Style = th.SpinnerStyle()

	// Detect terminal width
	width := cfg.UI.ChatWindowWidth
	if termWidth, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		if termWidth < width {
			width = termWidth
		}
	}
	// Reserve space for padding/borders if needed, but for now exact width is fine

	r, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)

	return &model{
		spinner:     s,
		glamour:     r,
		theme:       th,
		config:      cfg,
		width:       width,
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

func (m *model) renderMarkdown(text string) string {
	out, err := m.glamour.Render(text)
	if err != nil {
		return text
	}
	return out
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
			out := m.renderMarkdown(m.streamingText)
			cmds = append(cmds, tea.Println(out))
			m.streamingText = ""
		}

		// Initialize new tool state
		ts := &toolState{
			callID:  e.CallID,
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

			// Remove from order slice (zero-alloc in-place filter)
			n := 0
			for _, id := range m.toolOrder {
				if id != e.CallID {
					m.toolOrder[n] = id
					n++
				}
			}
			m.toolOrder = m.toolOrder[:n]

			return m, tea.Println(output)
		}
		return m, nil

	case domain.DoneEvent:
		if m.streamingText != "" {
			out := m.renderMarkdown(m.streamingText)
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
		out := m.renderMarkdown(m.streamingText)
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
