package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// streamTickMsg indicates it's time to drain the text queue.
type streamTickMsg struct{}

// eventMsg wraps a domain.Event for Bubble Tea.
type eventMsg struct {
	event domain.Event
}

type toolState struct {
	id      string
	display domain.ToolDisplay
	output  string
	status  ToolStatus
	err     string
}

// Model is the Bubble Tea model for the UI engine.
type Model struct {
	events <-chan domain.Event
	stream *Stream
	theme  *Theme
	width  int
	height int

	// Thinking state
	isThinking bool
	thinkStart time.Time
	thinkEnd   time.Time
	spinner    spinner.Model

	// Tool state
	activeTool *toolState

	// Smooth streaming state
	textQueue string
}

// NewModel creates a new UI engine model.
func NewModel(events <-chan domain.Event, width int) *Model {
	cfg := config.DefaultConfig()
	theme := NewTheme(cfg.UI)
	renderer, _ := NewGlamourRenderer(width)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(theme.primary)

	return &Model{
		events:  events,
		stream:  NewStream(renderer),
		theme:   theme,
		width:   width,
		height:  40, // Default height
		spinner: s,
	}
}

// Init initializes the model.
func (m *Model) Init() tea.Cmd {
	return m.waitForEvent()
}

// Update handles messages and returns mutated model and commands.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}

	case spinner.TickMsg:
		if m.isThinking || (m.activeTool != nil && m.activeTool.status == StatusRunning) {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case streamTickMsg:
		if m.textQueue == "" {
			return m, nil
		}

		// Drain up to 4 runes
		count := 0
		byteOffset := 0
		for count < 4 && byteOffset < len(m.textQueue) {
			_, size := utf8.DecodeRuneInString(m.textQueue[byteOffset:])
			byteOffset += size
			count++
		}

		chunk := m.textQueue[:byteOffset]
		m.textQueue = m.textQueue[byteOffset:]

		safe, err := m.stream.Append(chunk)
		if err == nil && len(safe) > 0 {
			cmds = append(cmds, m.flushHistory(safe))
		}

		if m.textQueue != "" {
			cmds = append(cmds, m.streamTick())
		}
		cmds = append(cmds, m.waitForEvent())
		return m, tea.Batch(cmds...)

	case eventMsg:
		// Logic to end thinking if any substantive event arrives
		if m.isThinking {
			switch msg.event.(type) {
			case domain.TextEvent, domain.ToolStartEvent, domain.DoneEvent:
				m.isThinking = false
				m.thinkEnd = time.Now()
				duration := m.thinkEnd.Sub(m.thinkStart).Round(time.Second)
				style := lipgloss.NewStyle().Foreground(m.theme.success)
				checkmark := style.Render("✔")
				cmds = append(cmds, tea.Printf("\n  %s Thought for %v\n", checkmark, style.Render(duration.String())))
			}
		}

		switch ev := msg.event.(type) {
		case domain.ThinkingEvent:
			m.isThinking = true
			m.thinkStart = time.Now()
			cmds = append(cmds, m.spinner.Tick)

		case domain.TextEvent:
			wasEmpty := m.textQueue == ""
			m.textQueue += ev.Text
			if wasEmpty && m.textQueue != "" {
				cmds = append(cmds, m.streamTick())
			}

		case domain.ToolStartEvent:
			m.activeTool = &toolState{
				id:      ev.CallID,
				display: ev.Display,
				status:  StatusRunning,
			}
			cmds = append(cmds, m.spinner.Tick)

		case domain.ToolStreamEvent:
			if m.activeTool != nil && m.activeTool.id == ev.CallID {
				m.activeTool.output += ev.Chunk
			}

		case domain.ToolEndEvent:
			if m.activeTool != nil && m.activeTool.id == ev.CallID {
				m.activeTool.status = StatusSuccess
				if ev.Error != "" {
					m.activeTool.status = StatusError
					m.activeTool.err = ev.Error
				}
				// Render and Printf
				cmds = append(cmds, tea.Printf("%s", m.renderTool(m.activeTool)))
				m.activeTool = nil
			}

		case domain.DoneEvent:
			return m, tea.Quit
		}

		cmds = append(cmds, m.waitForEvent())
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// View computes the transient bottom-bar string.
func (m *Model) View() string {
	var sb strings.Builder

	if m.isThinking {
		duration := time.Since(m.thinkStart).Round(time.Second)
		blueStyle := lipgloss.NewStyle().Foreground(m.theme.primary)
		sb.WriteString(blueStyle.Render(fmt.Sprintf("\n %s Thinking for %v\n", m.spinner.View(), duration)))
	}

	if m.activeTool != nil {
		sb.WriteString("\n")
		sb.WriteString(m.renderTool(m.activeTool))
		sb.WriteString("\n")
	}

	sb.WriteString(m.stream.Pending())

	return TruncateWithIndicator(sb.String(), m.height)
}

func (m *Model) waitForEvent() tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-m.events
		if !ok {
			return nil
		}
		return eventMsg{event: ev}
	}
}

func (m *Model) flushHistory(blocks []string) tea.Cmd {
	if len(blocks) == 0 {
		return nil
	}
	var combined string
	for _, b := range blocks {
		combined += b
	}
	return tea.Printf("%s", combined)
}

func (m *Model) renderTool(ts *toolState) string {
	var content string
	prefix := m.spinner.View()
	if ts.status == StatusSuccess {
		prefix = lipgloss.NewStyle().Foreground(m.theme.success).Render("✔")
	} else if ts.status == StatusError {
		prefix = lipgloss.NewStyle().Foreground(m.theme.err).Render("✘")
	}

	switch d := ts.display.(type) {
	case domain.ShellDisplay:
		content = RenderShell(m.width, 12, m.theme, d, ts.output, ts.status, ts.err, prefix)
	case domain.DiffDisplay:
		content = RenderDiff(m.width, m.theme, d, ts.status, ts.err, prefix)
	case domain.StringDisplay:
		content = RenderString(m.theme, d, ts.status, ts.err, prefix)
	}

	return m.theme.Box(content, m.width, ts.status)
}

func (m *Model) streamTick() tea.Cmd {
	return tea.Tick(time.Millisecond*16, func(t time.Time) tea.Msg {
		return streamTickMsg{}
	})
}
