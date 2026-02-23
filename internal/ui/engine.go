package ui

import (
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
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
	isThinking       bool
	thinkStart       time.Time
	thinkingDuration time.Duration
	spinner          spinner.Model

	// Tool state
	activeTools map[string]*toolState
	toolOrder   []string

	// Single source of truth queue
	queue       []domain.Event
	isStreaming bool

	pendingOutput []string
	flush         func(string) tea.Cmd
	isWaiting     bool
	isQuitting    bool
}

// Option is a functional option for configuring the Model.
type Option func(*Model)

// WithFlush sets the flush function for the model.
func WithFlush(f func(string) tea.Cmd) Option {
	return func(m *Model) {
		m.flush = f
	}
}

// NewModel creates a new UI engine model.
func NewModel(events <-chan domain.Event, cfg config.UIConfig, opts ...Option) *Model {
	theme := NewTheme(cfg)

	// Detect terminal width
	detectedWidth, _, err := term.GetSize(int(os.Stdout.Fd()))
	width := cfg.ChatWindowWidth

	if err == nil && detectedWidth < width {
		width = detectedWidth
	}

	renderer, _ := NewGlamourRenderer(width)

	s := spinner.New()
	s.Spinner = spinner.Spinner{
		Frames: []string{"⣾ ", "⣷ ", "⣯ ", "⣟ ", "⡿ ", "⢿ ", "⣻ ", "⣽ "},
		FPS:    time.Second / 10,
	}
	s.Style = lipgloss.NewStyle().Foreground(theme.primary)

	m := &Model{
		events:      events,
		stream:      NewStream(renderer),
		theme:       theme,
		width:       width,
		height:      40, // Default height
		spinner:     s,
		activeTools: make(map[string]*toolState),
		flush: func(content string) tea.Cmd {
			return tea.Printf("%s", content)
		},
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
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
			m.queue = nil
			m.isStreaming = false
			// DISCARD buffered text, only flush currently visible tail (stream buffer)
			safe, _ := m.stream.Flush()
			m.pendingOutput = append(m.pendingOutput, safe...)

			m.isThinking = false
			m.activeTools = make(map[string]*toolState)
			m.toolOrder = nil
			return m.finalize([]tea.Cmd{tea.Quit})
		}

	case spinner.TickMsg:
		if m.isThinking || len(m.activeTools) > 0 {
			if m.isThinking { // Keep this check to only update thinkingDuration when actually thinking
				m.thinkingDuration = time.Since(m.thinkStart).Round(time.Second)
			}
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m.finalize([]tea.Cmd{cmd})
		}
		return m.finalize(nil)

	case streamTickMsg:
		m.isStreaming = false
		return m.processQueue()

	case eventMsg:
		m.isWaiting = false
		m.queue = append(m.queue, msg.event)
		if m.isStreaming {
			return m, nil
		}
		return m.processQueue()
	}
	return m.finalize(cmds)
}

func (m *Model) finalize(cmds []tea.Cmd) (tea.Model, tea.Cmd) {
	if len(m.pendingOutput) > 0 {
		content := strings.Join(m.pendingOutput, "")
		m.pendingOutput = nil
		cmds = append(cmds, m.flush(content))
	}

	// ALWAYS listen for the next event UNLESS we are quitting or already waiting.
	if !m.isWaiting && !m.isQuitting {
		cmds = append(cmds, m.waitForEvent())
	}

	if len(cmds) == 0 {
		return m, nil
	}
	return m, tea.Batch(cmds...)
}

func (m *Model) processQueue() (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	for len(m.queue) > 0 {
		ev := m.queue[0]

		if te, ok := ev.(domain.TextEvent); ok {
			// If empty text event, just pop and continue
			if len(te.Text) == 0 {
				m.queue = m.queue[1:]
				continue
			}

			// Pre-split the text into chunks of up to 4 runes
			var chunks []domain.TextEvent
			text := te.Text
			for len(text) > 0 {
				count := 0
				byteOffset := 0
				for count < 4 && byteOffset < len(text) {
					_, size := utf8.DecodeRuneInString(text[byteOffset:])
					byteOffset += size
					count++
				}
				chunks = append(chunks, domain.TextEvent{Text: text[:byteOffset]})
				text = text[byteOffset:]
			}

			// Sub the big event out for the smaller ones in that exact slot
			if len(chunks) > 1 {
				newQueue := make([]domain.Event, 0, len(chunks)+len(m.queue)-1)
				for _, c := range chunks {
					newQueue = append(newQueue, c) // Convert explicitly
				}
				newQueue = append(newQueue, m.queue[1:]...)
				m.queue = newQueue

				// Update te to the first chunk
				te = m.queue[0].(domain.TextEvent)
			} else {
				// Ensure te refers to the exact single chunk
				te = chunks[0]
			}

			tm, cmd := m.handleEvent(te)
			m = tm.(*Model)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}

			// Text chunk finished, pop
			m.queue = m.queue[1:]

			// If there's another TextEvent immediately following, wait for a tick
			if len(m.queue) > 0 {
				if _, nextIsText := m.queue[0].(domain.TextEvent); nextIsText {
					m.isStreaming = true
					cmds = append(cmds, m.streamTick())
					return m.finalize(cmds) // Need to wait for tick
				}
			}

			continue
		}

		// Non-text event: pop and process
		m.queue = m.queue[1:]
		tm, cmd := m.handleEvent(ev)
		m = tm.(*Model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		// Continue loop to process next event immediately
	}

	return m.finalize(cmds)
}

func (m *Model) flushAll() {
	safe, err := m.stream.Flush()
	if err != nil {
		m.pendingOutput = append(m.pendingOutput, m.stream.RawBuffer())
		m.stream.ClearBuffer()
	} else if len(safe) > 0 {
		m.pendingOutput = append(m.pendingOutput, safe...)
	}

	// Force graduate everything remaining (e.g. on DoneEvent)
	for len(m.toolOrder) > 0 {
		id := m.toolOrder[0]
		ts := m.activeTools[id]
		m.pendingOutput = append(m.pendingOutput, m.renderTool(ts)+"\n")
		delete(m.activeTools, id)
		m.toolOrder = m.toolOrder[1:]
	}
}

func (m *Model) flushFinishedTools() {
	for len(m.toolOrder) > 0 {
		id := m.toolOrder[0]
		ts := m.activeTools[id]

		if ts.status == StatusRunning {
			break
		}

		// Flush any pending text before tool box to preserve sequence
		safe, err := m.stream.Flush()
		if err != nil {
			m.pendingOutput = append(m.pendingOutput, m.stream.RawBuffer())
			m.stream.ClearBuffer()
		} else if len(safe) > 0 {
			m.pendingOutput = append(m.pendingOutput, safe...)
		}

		m.pendingOutput = append(m.pendingOutput, m.renderTool(ts)+"\n")
		delete(m.activeTools, id)
		m.toolOrder = m.toolOrder[1:]
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

			style := lipgloss.NewStyle().Foreground(m.theme.success)
			checkmark := style.Render("✔")
			m.pendingOutput = append(m.pendingOutput, fmt.Sprintf("\n  %s Thought for %v\n", checkmark, style.Render(finalDuration.String())))
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
		safe, err := m.stream.Append(ev.Text)
		if err != nil {
			// Fallback: append raw chunk if rendering fails
			m.pendingOutput = append(m.pendingOutput, ev.Text)
		} else if len(safe) > 0 {
			m.pendingOutput = append(m.pendingOutput, safe...)
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

		ts := &toolState{
			id:      ev.CallID,
			display: ev.Display,
			status:  StatusRunning,
		}
		m.activeTools[ev.CallID] = ts
		m.toolOrder = append(m.toolOrder, ev.CallID)
		cmds = append(cmds, m.spinner.Tick)

	case domain.ToolStreamEvent:
		if ts, ok := m.activeTools[ev.CallID]; ok {
			ts.output += ev.Chunk
		}

	case domain.ToolEndEvent:
		if ts, ok := m.activeTools[ev.CallID]; ok {
			ts.status = StatusSuccess
			if ev.Error != "" {
				ts.status = StatusError
				ts.err = ev.Error
			}

			m.flushFinishedTools()
		}

	case domain.DoneEvent:
		m.isQuitting = true
		m.flushAll()
		return m.finalize([]tea.Cmd{tea.Quit})
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

	for _, id := range m.toolOrder {
		if ts, ok := m.activeTools[id]; ok {
			sb.WriteString(m.renderTool(ts))
			sb.WriteString("\n")
		}
	}

	sb.WriteString(m.stream.Pending())

	return TruncateWithIndicator(sb.String(), m.height)
}

func (m *Model) waitForEvent() tea.Cmd {
	m.isWaiting = true
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
