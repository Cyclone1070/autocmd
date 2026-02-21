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

// flushSignalMsg is a discrete signal emitted by the central flusher.
type flushSignalMsg struct {
	content string
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
	isThinking       bool
	thinkStart       time.Time
	thinkingDuration time.Duration
	spinner          spinner.Model

	// Tool state
	activeTool *toolState

	// Smooth streaming state
	textQueue     string
	eventBuffer   []domain.Event
	pendingOutput []string
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
			m.eventBuffer = nil
			// DISCARD textQueue, only flush currently visible tail (stream buffer)
			safe, _ := m.stream.Flush()
			m.pendingOutput = append(m.pendingOutput, safe...)

			m.textQueue = ""
			m.isThinking = false
			m.activeTool = nil
			return m.finalize([]tea.Cmd{tea.Quit})
		}

	case spinner.TickMsg:
		if m.isThinking || (m.activeTool != nil && m.activeTool.status == StatusRunning) {
			if m.isThinking {
				m.thinkingDuration = time.Since(m.thinkStart).Round(time.Second)
			}
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m.finalize([]tea.Cmd{cmd})
		}
		return m.finalize(nil)

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
		if err != nil {
			// Fallback: append raw chunk if rendering fails
			m.pendingOutput = append(m.pendingOutput, chunk)
		} else if len(safe) > 0 {
			m.pendingOutput = append(m.pendingOutput, safe...)
		}

		if m.textQueue != "" {
			cmds = append(cmds, m.streamTick())
		} else if len(m.eventBuffer) > 0 {
			for len(m.eventBuffer) > 0 {
				ev := m.eventBuffer[0]
				m.eventBuffer = m.eventBuffer[1:]
				tm, cmd := m.handleEvent(ev)
				m = tm.(*Model)
				cmds = append(cmds, cmd)
				if m.textQueue != "" {
					break
				}
			}
		}
		return m.finalize(cmds)

	case eventMsg:
		if m.textQueue != "" || len(m.eventBuffer) > 0 {
			m.eventBuffer = append(m.eventBuffer, msg.event)
			return m.finalize(nil)
		}
		tm, cmd := m.handleEvent(msg.event)
		m = tm.(*Model)
		return m.finalize([]tea.Cmd{cmd})
	}
	return m.finalize(cmds)
}

func (m *Model) finalize(cmds []tea.Cmd) (tea.Model, tea.Cmd) {
	if len(m.pendingOutput) > 0 {
		content := strings.Join(m.pendingOutput, "")
		m.pendingOutput = nil
		cmds = append(cmds,
			tea.Printf("%s", content),
			func() tea.Msg { return flushSignalMsg{content: content} },
		)
	}

	// ALWAYS listen for the next event UNLESS we are quitting.
	cmds = append(cmds, m.waitForEvent())
	return m, tea.Batch(cmds...)
}

func (m *Model) flushAll() {
	if m.textQueue != "" {
		safe, err := m.stream.Append(m.textQueue)
		if err != nil {
			m.pendingOutput = append(m.pendingOutput, m.textQueue)
		} else {
			m.pendingOutput = append(m.pendingOutput, safe...)
		}
		m.textQueue = ""
	}
	safe, err := m.stream.Flush()
	if err != nil {
		m.pendingOutput = append(m.pendingOutput, m.stream.RawBuffer())
		m.stream.ClearBuffer()
	} else {
		m.pendingOutput = append(m.pendingOutput, safe...)
	}
}

// handleEvent processes a domain event and returns commands.
func (m *Model) handleEvent(event domain.Event) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Logic to end thinking if any substantive event arrives
	if m.isThinking {
		switch event.(type) {
		case domain.TextEvent, domain.ToolStartEvent, domain.DoneEvent:
			m.isThinking = false
			finalDuration := time.Since(m.thinkStart).Round(time.Second)

			// Flush any pending text before printing duration to ensure order
			safe, err := m.stream.Flush()
			if err != nil {
				m.pendingOutput = append(m.pendingOutput, m.stream.RawBuffer())
				m.stream.ClearBuffer()
			} else {
				m.pendingOutput = append(m.pendingOutput, safe...)
			}

			duration := finalDuration
			style := lipgloss.NewStyle().Foreground(m.theme.success)
			checkmark := style.Render("✔")
			m.pendingOutput = append(m.pendingOutput, fmt.Sprintf("\n  %s Thought for %v\n", checkmark, style.Render(duration.String())))
		}
	}

	switch ev := event.(type) {
	case domain.ThinkingEvent:
		// Flush any pending text before switching to thinking state
		safe, err := m.stream.Flush()
		if err != nil {
			m.pendingOutput = append(m.pendingOutput, m.stream.RawBuffer())
			m.stream.ClearBuffer()
		} else {
			m.pendingOutput = append(m.pendingOutput, safe...)
		}

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
		// Flush any pending text before starting a tool
		safe, err := m.stream.Flush()
		if err != nil {
			m.pendingOutput = append(m.pendingOutput, m.stream.RawBuffer())
			m.stream.ClearBuffer()
		} else {
			m.pendingOutput = append(m.pendingOutput, safe...)
		}

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

			// Flush any pending text before printing tool output box
			safe, err := m.stream.Flush()
			if err != nil {
				m.pendingOutput = append(m.pendingOutput, m.stream.RawBuffer())
				m.stream.ClearBuffer()
			} else {
				m.pendingOutput = append(m.pendingOutput, safe...)
			}

			// Render and Printf
			m.pendingOutput = append(m.pendingOutput, m.renderTool(m.activeTool))
			m.activeTool = nil
		}

	case domain.DoneEvent:
		m.flushAll()
		return m, tea.Quit
	}

	return m, tea.Batch(cmds...)
}

// View computes the transient bottom-bar string.
func (m *Model) View() string {
	var sb strings.Builder

	if m.isThinking {
		duration := m.thinkingDuration
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
