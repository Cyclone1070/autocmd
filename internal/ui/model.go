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
	spinner    spinner.Model
	glamour    *glamour.TermRenderer
	theme      *theme
	config     *config.Config
	width      int
	termHeight int // For overflow indicator

	// New State tracking
	streamingMd *StreamingMarkdown // Handles text buffering strings
	tools       []*toolState       // Ordered list of all tools (active + waiting flush)

	// Keep
	thinking bool
	runState runState
}

type toolState struct {
	callID      string
	display     domain.ToolDisplay
	status      toolStatus
	err         string
	shellOutput strings.Builder
}

type toolStatus int

const (
	statusRunning toolStatus = iota
	statusSuccess
	statusError
)

type runState int

const (
	stateRunning runState = iota
	stateDone
	stateCancelled
)

func newModel(cfg *config.Config) (*model, error) {
	th := newTheme(cfg.UI)
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = th.SpinnerStyle()

	// Detect terminal size
	width := cfg.UI.ChatWindowWidth
	height := 24 // default fallback
	if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		if w < width {
			width = w
		}
		height = h
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize markdown renderer: %w", err)
	}

	md := NewStreamingMarkdown(r)

	return &model{
		spinner:     s,
		glamour:     r,
		theme:       th,
		config:      cfg,
		width:       width,
		termHeight:  height,
		streamingMd: md,
		tools:       make([]*toolState, 0),
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
		var cmd tea.Cmd
		var newSpinner spinner.Model
		newSpinner, cmd = m.spinner.Update(ev)
		m.spinner = newSpinner

		// Note: We don't need to force render here for tool spinners because
		// in inline mode, the View() is reprinted on every update automatically by Bubble Tea framework
		// assuming standard Update/View cycle. With Altscreen/Viewport we needed it because content was cached.
		// Wait, View() IS called after every Update. Standard Bubble Tea behavior.

		return m, cmd

	case tea.KeyMsg:
		if ev.Type == tea.KeyCtrlC {
			m.runState = stateCancelled

			// Build final output with proper padding (same as DoneEvent)
			var parts []string

			// 1. Flush pending text
			textFlush, _ := m.streamingMd.Flush()
			if textFlush != "" {
				parts = append(parts, strings.TrimRight(textFlush, "\n"))
			}

			// 2. Flush remaining tools
			for _, ts := range m.tools {
				parts = append(parts, m.viewTool(ts))
			}
			m.tools = nil

			// 3. Add cancelled status bar
			parts = append(parts, m.statusBar())

			// Join with double newline for consistent padding
			finalOutput := "\n" + strings.Join(parts, "\n\n")
			return m, tea.Sequence(tea.Println(finalOutput), tea.Quit)
		}

	case tea.WindowSizeMsg:
		m.width = min(ev.Width, m.config.UI.ChatWindowWidth)
		m.termHeight = ev.Height

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
		m.streamingMd.SetRenderer(r)
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

		// Flush any completed tools at the front of the queue
		cmds := m.flushCompletedTools()

		// Append text and get flushable blocks
		flushedBlocks, err := m.streamingMd.Append(e.Text)
		if err != nil {
			// Log error but continue?
			cmds = append(cmds, tea.Println(fmt.Sprintf("Error rendering markdown: %v", err)))
		}

		for _, block := range flushedBlocks {
			cmds = append(cmds, tea.Println(block))
		}

		if len(cmds) > 0 {
			return m, tea.Sequence(cmds...)
		}
		return m, nil

	case domain.ToolStartEvent:
		m.thinking = false

		// Flush completed tools first (heuristic: new tool implies old ones might be done done)
		var cmds []tea.Cmd
		cmds = append(cmds, m.flushCompletedTools()...)

		// Note: We do NOT force flush text here. We rely on StreamingMarkdown logic.
		// If the text block was "Uncertain", it remains "Uncertain" until a new block starts
		// or we force flush.

		textFlush, _ := m.streamingMd.Flush()
		if textFlush != "" {
			cmds = append(cmds, tea.Println(textFlush))
		}

		// Initialize new tool
		ts := &toolState{
			callID:  e.CallID,
			display: e.Display,
			status:  statusRunning,
		}
		m.tools = append(m.tools, ts)

		cmds = append(cmds, m.spinner.Tick)
		return m, tea.Sequence(cmds...)

	case domain.ToolStreamEvent:
		// Find tool in slice
		for _, ts := range m.tools {
			if ts.callID == e.CallID {
				ts.shellOutput.WriteString(e.Chunk)
				break
			}
		}
		return m, nil

	case domain.ToolEndEvent:
		// Mark tool as done
		for _, ts := range m.tools {
			if ts.callID == e.CallID {
				if e.Error != "" {
					ts.status = statusError
					ts.err = e.Error
				} else {
					ts.status = statusSuccess
				}
				break
			}
		}

		// Attempt flush from front
		cmds := m.flushCompletedTools()
		if len(cmds) > 0 {
			return m, tea.Sequence(cmds...)
		}
		return m, nil

	case domain.DoneEvent:
		m.runState = stateDone

		// Build final output with proper padding
		var parts []string

		// 1. Flush pending text
		textFlush, _ := m.streamingMd.Flush()
		if textFlush != "" {
			parts = append(parts, strings.TrimRight(textFlush, "\n"))
		}

		// 2. Flush remaining tools
		for _, ts := range m.tools {
			parts = append(parts, m.viewTool(ts))
		}
		m.tools = nil // Clear

		// 3. Add final status bar
		parts = append(parts, m.statusBar())

		// Join with double newline for consistent padding
		// Prepend \n to ensure padding from previously flushed content (tools, text)
		finalOutput := "\n" + strings.Join(parts, "\n\n")
		return m, tea.Sequence(tea.Println(finalOutput), tea.Quit)
	}

	return m, nil
}

// flushCompletedTools checks the front of the tool queue.
// If the first tool is complete, it flushes it and repeats for subsequent tools.
// Returns a list of tea.Println commands.
func (m *model) flushCompletedTools() []tea.Cmd {
	var cmds []tea.Cmd

	// While list is not empty AND first item is not running
	for len(m.tools) > 0 && m.tools[0].status != statusRunning {
		// Pop front
		tool := m.tools[0]
		m.tools = m.tools[1:]

		// Create flush command
		output := m.viewTool(tool)
		cmds = append(cmds, tea.Println(output))
	}

	return cmds
}

func (m *model) View() string {
	// When done or cancelled, we've already flushed everything via tea.Println
	// Return empty to prevent duplicate rendering and extra whitespace
	if m.runState == stateDone || m.runState == stateCancelled {
		return ""
	}

	var parts []string

	// 1. Pending markdown (last uncertain block)
	if pending := m.streamingMd.Pending(); pending != "" {
		pending = m.truncateWithIndicator(pending)
		parts = append(parts, pending)
	}

	// 2. All remaining tools (running + waiting-to-flush)
	for _, t := range m.tools {
		parts = append(parts, m.viewTool(t))
	}

	// 3. Status bar
	// Ensure we have at least one element before status bar to force padding
	// if we are in "Done" state (where other parts are empty).
	if len(parts) == 0 && m.runState == stateDone {
		parts = append(parts, "")
	}
	parts = append(parts, m.statusBar())

	return strings.Join(parts, "\n\n")
}

// Overflow indicator - show only bottom portion if too tall
func (m *model) truncateWithIndicator(content string) string {
	lines := strings.Split(content, "\n")
	// Calculate available space: total height - tool buffer - status
	// But we don't know tool buffer height easily without rendering.
	// We'll use a conservative heuristic: leave 5 lines.
	maxLines := m.termHeight - 5

	if maxLines < 5 {
		maxLines = 5 // Minimum visibility
	}

	if len(lines) <= maxLines {
		return content
	}

	overflow := len(lines) - maxLines
	visible := lines[overflow:]
	header := fmt.Sprintf("\n  ↑ (%d lines temporarily truncated)", overflow)
	return header + "\n" + strings.Join(visible, "\n")
}

func (m *model) statusBar() string {
	// Hardcoded context window info for now
	contextInfo := m.theme.Muted("Context: 42%")

	var left string
	switch m.runState {
	case stateDone:
		left = fmt.Sprintf("%s Done", m.theme.Success("✓"))
	case stateCancelled:
		left = fmt.Sprintf("%s Cancelled", m.theme.Error("✗"))
	default:
		status := "Generating"
		if m.thinking {
			status = "Thinking"
		}
		left = fmt.Sprintf("%s %s", m.spinner.View(), m.theme.Primary(status))
	}

	// Calculate padding to right-align context info
	// Approximate: width - left length - right length
	// Since ANSI codes mess up length, use a fixed padding approach
	padding := m.width - 20 // rough estimate leaving room for both sides
	if padding < 1 {
		padding = 1
	}

	return fmt.Sprintf("%s%*s", left, padding, contextInfo)
}
