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
	width          int
	activeTools    map[string]*toolState // active tool executions
	completedTools []*toolState          // completed tools waiting to be flushed
	toolOrder      []string              // order of active tools for stable rendering
	thinking       bool                  // true if waiting for LLM response
	streamingText  string                // current markdown text
	renderedText   string                // cached rendered markdown
	textDirty      bool                  // true if streamingText or width changed
	maxViewHeight  int                   // high water mark for View() height
	done           bool                  // true if workflow is finished
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

func newModel(cfg *config.Config) (*model, error) {
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

	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize markdown renderer: %w", err)
	}

	return &model{
		spinner:     s,
		glamour:     r,
		theme:       th,
		config:      cfg,
		width:       width,
		activeTools: make(map[string]*toolState),
	}, nil
}

func (m *model) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m *model) Update(teaMsg tea.Msg) (tea.Model, tea.Cmd) {
	switch ev := teaMsg.(type) {
	case msg:
		return m.handleEvent(ev.Event)

	case spinner.TickMsg:
		// Always tick spinner as bottom bar is always visible
		var cmd tea.Cmd
		var newSpinner spinner.Model
		newSpinner, cmd = m.spinner.Update(ev)
		m.spinner = newSpinner
		return m, cmd

	case tea.KeyMsg:
		switch ev.String() {
		case "ctrl+c":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = min(ev.Width, m.config.UI.ChatWindowWidth)
		r, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(m.width),
		)
		if err != nil {
			return m, tea.Sequence(
				tea.Println(fmt.Sprintf("Fatal: glamour re-init failed: %v", err)),
				tea.Quit,
			)
		}
		m.glamour = r
		m.textDirty = true
		if cmd := m.render(); cmd != nil {
			return m, cmd
		}
		return m, nil
	}
	return m, nil
}

func (m *model) render() tea.Cmd {
	if m.textDirty {
		out, err := m.glamour.Render(m.streamingText)
		if err != nil {
			return tea.Sequence(
				tea.Println(fmt.Sprintf("Fatal: render failed: %v", err)),
				tea.Quit,
			)
		}
		m.renderedText = out
		m.textDirty = false
	}
	return nil
}

func (m *model) handleEvent(ev domain.Event) (tea.Model, tea.Cmd) {
	switch e := ev.(type) {
	case domain.ThinkingEvent:
		// Flush completed tools first
		var cmds []tea.Cmd
		if flushCmd := m.flushCompleted(); flushCmd != nil {
			cmds = append(cmds, flushCmd)
		}

		m.thinking = true
		cmds = append(cmds, m.spinner.Tick)
		return m, tea.Batch(cmds...)

	case domain.TextEvent:
		// Flush completed tools first
		var cmds []tea.Cmd
		if flushCmd := m.flushCompleted(); flushCmd != nil {
			cmds = append(cmds, flushCmd)
		}

		m.thinking = false
		m.streamingText += e.Text
		m.textDirty = true
		if cmd := m.render(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if len(cmds) > 0 {
			return m, tea.Sequence(cmds...)
		}
		return m, nil

	case domain.ToolStartEvent:
		var cmds []tea.Cmd
		m.thinking = false

		// Flush completed tools first
		if flushCmd := m.flushCompleted(); flushCmd != nil {
			cmds = append(cmds, flushCmd)
		}

		// Flush streaming text if any
		if m.streamingText != "" {
			if cmd := m.render(); cmd != nil {
				cmds = append(cmds, cmd)
			}
			cmds = append(cmds, tea.Println(m.renderedText))
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

			// Move to completed tools (stay in View) instead of flushing immediately
			m.completedTools = append(m.completedTools, ts)

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
		}
		return m, nil

	case domain.DoneEvent:
		m.done = true
		var cmds []tea.Cmd

		// Flush completed tools
		if flushCmd := m.flushCompleted(); flushCmd != nil {
			cmds = append(cmds, flushCmd)
		}

		if m.streamingText != "" {
			if cmd := m.render(); cmd != nil {
				// This should not happen, render() only returns tea.Quit on fatal error
				// If it does, we should return it immediately.
				return m, cmd
			}
			cmds = append(cmds, tea.Println(m.renderedText))
		}
		cmds = append(cmds, tea.Quit)
		return m, tea.Sequence(cmds...)
	}

	return m, nil
}

// flushCompleted takes all completed tools and creates tea.Println commands for them.
// It clears the completedTools slice after processing.
func (m *model) flushCompleted() tea.Cmd {
	if len(m.completedTools) == 0 {
		return nil
	}

	var cmds []tea.Cmd
	for _, ts := range m.completedTools {
		output := m.viewTool(ts)
		cmds = append(cmds, tea.Println(output))
	}
	m.completedTools = nil // Clear the slice
	m.maxViewHeight = 0    // Reset height tracking as content moved to history

	return tea.Batch(cmds...)
}

func (m *model) View() string {
	if m.done {
		return ""
	}

	// 1. Build main content
	var mainContent string
	if m.streamingText != "" {
		mainContent = strings.TrimRight(m.renderedText, "\n")
	} else {
		// Render tools: Completed ones first, then active ones
		var toolViews []string

		for _, ts := range m.completedTools {
			toolViews = append(toolViews, m.viewTool(ts))
		}

		for _, id := range m.toolOrder {
			if ts, ok := m.activeTools[id]; ok {
				toolViews = append(toolViews, m.viewTool(ts))
			}
		}

		if len(toolViews) > 0 {
			mainContent = strings.TrimRight(strings.Join(toolViews, "\n"), "\n")
		}
	}

	// 2. Build bottom bar
	statusText := "Generating"
	if m.thinking {
		statusText = "Thinking"
	}
	bottomBar := fmt.Sprintf("%s %s", m.spinner.View(), statusText)

	// 3. Calculate Visual Heights and Middle Padding
	contentHeight := 0
	if mainContent != "" {
		// Calculate height of content (including internal newlines)
		contentHeight = strings.Count(mainContent, "\n") + 1
	}

	barHeight := 1
	marginHeight := 0
	if mainContent != "" {
		marginHeight = 1 // for the \n\n margin
	}

	// Calculate current required height
	currentTotal := contentHeight + marginHeight + barHeight

	// Update High Water Mark
	if currentTotal > m.maxViewHeight {
		m.maxViewHeight = currentTotal
	}

	// Calculate Middle Padding needed to maintain max height
	padHeight := m.maxViewHeight - currentTotal
	if padHeight < 0 {
		padHeight = 0
	}
	padding := strings.Repeat("\n", padHeight)

	// 4. Compose: Content + Margin + Padding + Bar
	// Use explicit newlines for margin logic
	margin := ""
	if mainContent != "" {
		margin = "\n\n"
	}

	return mainContent + margin + padding + bottomBar
}
