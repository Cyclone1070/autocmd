package loop

import (
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// streamTickMsg indicates it's time to drain the text queue.
type streamTickMsg struct{}

// flushDoneMsg indicates that a terminal flush (tea.Printf) has completed.
type flushDoneMsg struct{}

// eventMsg wraps a domain.UIUpdate for Bubble Tea.
type eventMsg struct {
	update domain.UIUpdate
}

type toolState struct {
	id      string
	display domain.ToolDisplay
	output  string
	status  ui.ToolStatus
	err     string
}

// bus defines the communication contract for the UI engine.
type bus interface {
	UIUpdates() <-chan domain.UIUpdate
	SendAction(domain.Action)
}

// Model is the Bubble Tea model for the UI engine.
type Model struct {
	bus bus
	stream *Stream
	theme  *ui.Theme
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
	queue       []domain.UIUpdate
	isStreaming bool

	printQueue []string
	flush      func(string) tea.Cmd
	isWaiting  bool
	isDone     bool
}

// Option is a functional option for configuring the Model.
type Option func(*Model)
// WithFlush sets the flush function for the model.
func WithFlush(f func(string) tea.Cmd) Option {
	return func(m *Model) {
		m.flush = f
	}
}

// WithIsDark sets the dark mode flag for the model.
func WithIsDark(dark bool) Option {
	return func(m *Model) {
		m.stream.renderer = ui.NewGlamourRenderer(m.width, dark)
	}
}

// NewModel creates a new UI engine model.
func NewModel(b bus, themeCfg ui.ThemeConfig, chatWindowWidth int, opts ...Option) *Model {
	theme := ui.NewTheme(themeCfg)

	// Detect terminal size
	detectedWidth, detectedHeight, err := term.GetSize(int(os.Stdout.Fd()))
	width := chatWindowWidth
	height := 40 // Default fallback

	if err == nil {
		if detectedWidth < width {
			width = detectedWidth
		}
		height = detectedHeight
	}

	// Default renderer (auto-detect if possible, but safely)
	var isDark bool
	if term.IsTerminal(int(os.Stdout.Fd())) {
		isDark = lipgloss.HasDarkBackground()
	}
	renderer := ui.NewGlamourRenderer(width, isDark)

	s := spinner.New()
	s.Spinner = spinner.Spinner{
		Frames: []string{"⣾", "⣷", "⣯", "⣟", "⡿", "⢿", "⣻", "⣽"},
		FPS:    time.Second / 10,
	}
	s.Style = lipgloss.NewStyle().Foreground(theme.PrimaryColor())

	m := &Model{
		bus:         b,
		stream:      NewStream(renderer),
		theme:       theme,
		width:       width,
		height:      height,
		spinner:     s,
		activeTools: make(map[string]*toolState),
		flush: func(content string) tea.Cmd {
			return tea.Sequence(
				tea.Printf("%s", content),
				func() tea.Msg { return flushDoneMsg{} },
			)
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
			if m.bus != nil {
				m.bus.SendAction(domain.StopAction{})
			}
			return m.handleInterrupt()
		}

	case spinner.TickMsg:
		if m.isThinking || len(m.activeTools) > 0 {
			if m.isThinking { // Keep this check to only update thinkingDuration when actually thinking
				m.thinkingDuration = time.Since(m.thinkStart).Round(time.Second)
			}
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, tea.Batch(cmd, tea.Sequence(m.flushPrintQueue(), m.ensureEventListener()))
		}
		return m, tea.Sequence(m.flushPrintQueue(), m.ensureEventListener())

	case streamTickMsg:
		m.isStreaming = false
		tm, tcmd := m.processQueue()
		m = tm.(*Model)
		if tcmd != nil {
			cmds = append(cmds, tcmd)
		}
		if m.isDone && !m.isStreaming && len(m.queue) == 0 {
			flushCmd := m.flushPrintQueue()
			if flushCmd == nil && len(cmds) == 0 {
				return m, tea.Quit
			}
			if flushCmd != nil {
				cmds = append(cmds, flushCmd)
			}
			return m, tea.Batch(cmds...)
		}

	case eventMsg:
		m.isWaiting = false
		if msg.update == nil {
			if !m.isDone && !m.isStreaming {
				panic("unexpected nil UIUpdate: bus closed before DoneEvent or handshake")
			}
			return m, nil
		}
		m.queue = append(m.queue, msg.update)
		if !m.isStreaming {
			tm, tcmd := m.processQueue()
			m = tm.(*Model)
			if tcmd != nil {
				cmds = append(cmds, tcmd)
			}
		}

	case flushDoneMsg:
		if m.isDone && !m.isStreaming && len(m.queue) == 0 {
			return m, tea.Quit
		}
		return m, m.ensureEventListener()
	}
	if m.isDone && !m.isStreaming && len(m.queue) == 0 {
		flushCmd := m.flushPrintQueue()
		if flushCmd == nil && len(cmds) == 0 {
			return m, tea.Quit
		}
		if flushCmd != nil {
			cmds = append(cmds, flushCmd)
		}
		return m, tea.Batch(cmds...)
	}

	return m, tea.Batch(tea.Batch(cmds...), tea.Sequence(m.flushPrintQueue(), m.ensureEventListener()))
}

func (m *Model) handleInterrupt() (tea.Model, tea.Cmd) {
	m.isDone = true
	// 1. Flush thinking with error status
	m.flushThinking(ui.StatusError)

	// 2. Mark all active tools as cancelled/error and flush them
	for _, id := range m.toolOrder {
		if ts, ok := m.activeTools[id]; ok {
			if ts.status == ui.StatusRunning {
				ts.status = ui.StatusError
				ts.err = "Cancelled"
			}
		}
	}
	m.flushAll()

	// 3. Clear transient state
	m.isStreaming = false
	m.queue = nil

	flushCmd := m.flushPrintQueue()
	if flushCmd == nil {
		return m, tea.Quit
	}
	return m, flushCmd
}

func (m *Model) ensureEventListener() tea.Cmd {
	if m.isWaiting {
		return nil
	}
	return m.waitForEvent()
}

func (m *Model) flushPrintQueue() tea.Cmd {
	if len(m.printQueue) > 0 {
		content := strings.Join(m.printQueue, "")
		m.printQueue = nil
		return m.flush(strings.TrimRight(content, "\n"))
	}
	return nil
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
				newQueue := make([]domain.UIUpdate, 0, len(chunks)+len(m.queue)-1)
				for _, c := range chunks {
					newQueue = append(newQueue, c) 
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

			// If there are more events following (including DoneEvent), wait for a tick
			// to allow the terminal to render the current chunk before quitting or starting tools.
			if len(m.queue) > 0 {
				m.isStreaming = true
				cmds = append(cmds, m.streamTick())
				return m, tea.Batch(tea.Batch(cmds...), tea.Sequence(m.flushPrintQueue(), m.ensureEventListener()))
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

	if m.isDone && !m.isStreaming && len(m.queue) == 0 {
		flushCmd := m.flushPrintQueue()
		if flushCmd == nil && len(cmds) == 0 {
			return m, tea.Quit
		}
		if flushCmd != nil {
			cmds = append(cmds, flushCmd)
		}
		return m, tea.Batch(cmds...)
	}

	return m, tea.Batch(tea.Batch(cmds...), tea.Sequence(m.flushPrintQueue(), m.ensureEventListener()))
}

func (m *Model) flushAll() {
	flushed := m.stream.Flush()
	if len(flushed) > 0 {
		m.printQueue = append(m.printQueue, flushed...)
	}

	// Force graduate everything remaining (e.g. on DoneEvent)
	for len(m.toolOrder) > 0 {
		id := m.toolOrder[0]
		ts := m.activeTools[id]
		m.printQueue = append(m.printQueue, m.renderTool(ts))
		delete(m.activeTools, id)
		m.toolOrder = m.toolOrder[1:]
	}
}

func (m *Model) flushFinishedTools() {
	for len(m.toolOrder) > 0 {
		id := m.toolOrder[0]
		ts := m.activeTools[id]

		if ts.status == ui.StatusRunning {
			break
		}

		// Flush any pending text before tool box to preserve sequence
		flushed := m.stream.Flush()
		if len(flushed) > 0 {
			m.printQueue = append(m.printQueue, flushed...)
		}

		m.printQueue = append(m.printQueue, m.renderTool(ts))
		delete(m.activeTools, id)
		m.toolOrder = m.toolOrder[1:]
	}
}

// handleEvent processes a domain UIUpdate and returns commands.
func (m *Model) handleEvent(event domain.UIUpdate) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Logic to end thinking if any substantive event arrives
	if m.isThinking {
		switch event.(type) {
		case domain.TextEvent, domain.ToolStartEvent, domain.DoneEvent:
			m.flushThinking(ui.StatusSuccess)
		}
	}

	switch ev := event.(type) {
	case domain.ThinkingEvent:
		// Flush any pending text before switching to thinking state
		flushed := m.stream.Flush()
		m.printQueue = append(m.printQueue, flushed...)

		m.isThinking = true
		m.thinkStart = time.Now()
		cmds = append(cmds, m.spinner.Tick)

	case domain.TextEvent:
		flushed := m.stream.Append(ev.Text)
		if len(flushed) > 0 {
			m.printQueue = append(m.printQueue, flushed...)
		}

	case domain.ToolStartEvent:
		// Flush any pending text before starting a tool
		flushed := m.stream.Flush()
		m.printQueue = append(m.printQueue, flushed...)

		ts := &toolState{
			id:      ev.CallID,
			display: ev.Display,
			status:  ui.StatusRunning,
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
			ts.status = ui.StatusSuccess
			if ev.Error != "" {
				ts.status = ui.StatusError
				ts.err = ev.Error
			}

			m.flushFinishedTools()
		}

	case domain.DoneEvent:
		m.isDone = true
		m.flushAll()
		flushCmd := m.flushPrintQueue()
		if flushCmd == nil {
			return m, tea.Quit
		}
		return m, flushCmd
	}

	return m, tea.Batch(cmds...)
}

// View computes the transient bottom-bar string.
func (m *Model) View() string {
	var sb strings.Builder

	if m.isThinking {
		duration := m.thinkingDuration
		style := lipgloss.NewStyle().Foreground(m.theme.PrimaryColor())
		// Style the text separately because the spinner's View() includes a reset sequence
		// that would clear any outer styling if nested.
		fmt.Fprintf(&sb, "\n %s %s\n", m.spinner.View(), style.Render(fmt.Sprintf("Thinking for %v", duration)))
	}

	for _, id := range m.toolOrder {
		if ts, ok := m.activeTools[id]; ok {
			sb.WriteString(m.renderTool(ts))
		}
	}

	sb.WriteString(m.stream.Pending())

	res := strings.TrimRight(sb.String(), "\n")
	return ui.TruncateWithIndicator(res, m.height)
}

func (m *Model) flushThinking(status ui.ToolStatus) {
	if !m.isThinking {
		return
	}

	finalDuration := time.Since(m.thinkStart).Round(time.Second)
	if status == ui.StatusError && m.thinkingDuration > 0 {
		finalDuration = m.thinkingDuration
	}

	m.isThinking = false

	var prefix string
	var textColor lipgloss.AdaptiveColor

	if status == ui.StatusSuccess {
		prefix = lipgloss.NewStyle().Foreground(m.theme.SuccessColor()).Render("✔")
		textColor = m.theme.SuccessColor()
	} else {
		prefix = lipgloss.NewStyle().Foreground(m.theme.ErrorColor()).Render("✘")
		textColor = m.theme.ErrorColor()
	}

	style := lipgloss.NewStyle().Foreground(textColor)
	m.printQueue = append(m.printQueue, fmt.Sprintf("\n %s %s\n", prefix, style.Render(fmt.Sprintf("Thought for %v", finalDuration))))
}

func (m *Model) waitForEvent() tea.Cmd {
	m.isWaiting = true
	return func() tea.Msg {
		upd, ok := <-m.bus.UIUpdates()
		if !ok {
			return eventMsg{update: nil} // Signal channel closure
		}
		return eventMsg{update: upd}
	}
}

func (m *Model) renderTool(ts *toolState) string {
	var content string
	prefix := m.spinner.View()
	switch ts.status {
	case ui.StatusSuccess:
		prefix = lipgloss.NewStyle().Foreground(m.theme.SuccessColor()).Render("✔")
	case ui.StatusError:
		prefix = lipgloss.NewStyle().Foreground(m.theme.ErrorColor()).Render("✘")
	}

	switch d := ts.display.(type) {
	case domain.ShellDisplay:
		content = ui.RenderShell(m.width-2, 12, m.theme, d, ts.output, ts.status, ts.err, prefix)
	case domain.DiffDisplay:
		content = ui.RenderDiff(m.width-2, 12, m.theme, d, ts.status, ts.err, prefix)
	case domain.StringDisplay:
		content = ui.RenderString(m.theme, d, ts.status, ts.err, prefix)
	}

	return m.theme.Box(content, m.width-2, ts.status)
}

func (m *Model) streamTick() tea.Cmd {
	return tea.Tick(time.Millisecond*16, func(t time.Time) tea.Msg {
		return streamTickMsg{}
	})
}
