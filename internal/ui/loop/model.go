package loop

import (
	"log/slog"
	"strings"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	runesPerTick = 4
	boxWidthPad  = 2
)

type spinnerProvider interface {
	Frame(tick int) string
}

type thinkingRenderer interface {
	RenderThinking(status ui.ToolStatus, start time.Time, tick int, sp spinnerProvider) string
}

type toolRenderer interface {
	StatusPrefix(status ui.ToolStatus, frame string) string
	RenderString(d domain.StringDisplay, status ui.ToolStatus, err string, prefix string) string
	RenderDiff(d domain.DiffDisplay, status ui.ToolStatus, err string, prefix string) string
	RenderShell(d domain.ShellDisplay, output string, status ui.ToolStatus, err string, prefix string) string
	Box(content string, width int, status ui.ToolStatus) string
}

type stream interface {
	Append(text string) []string
	Flush() []string
	Pending() string
}

type animator interface {
	Enqueue(text string)
	NextChunk() (string, bool)
	FlushAll() string
}

type bus interface {
	UIUpdates() <-chan domain.UIUpdate
	SendAction(domain.Action)
}

type uiState int

const (
	stateIdle uiState = iota
	stateThinking
	stateStreaming
	stateTooling
	stateDone
)

type toolSlot struct {
	callID       string
	toolName     string
	display      domain.ToolDisplay
	status       ui.ToolStatus
	errorMsg     string
	streamOutput string
}

// Model is the Bubble Tea model for the prompt path streaming UI.
type Model struct {
	state   uiState
	bus     bus
	stream  stream
	animator animator
	tools   []toolSlot
	
	// DI Services
	thinkingRenderer thinkingRenderer
	toolRenderer     toolRenderer
	spinnerProvider  spinnerProvider

	width         int
	flushFn       func(content string) tea.Cmd
	thinkingStart time.Time
	spinnerFrame  int
}

// Option configures the Model.
type Option func(*Model)

// WithFlush sets the function called when content is flushed to history.
func WithFlush(fn func(content string) tea.Cmd) Option {
	return func(m *Model) {
		m.flushFn = fn
	}
}

// NewModel creates a new loop Model with strict DI.
func NewModel(
	b bus,
	tr thinkingRenderer,
	tlr toolRenderer,
	sp spinnerProvider,
	s stream,
	a animator,
	chatWindowWidth int,
	opts ...Option,
) *Model {
	m := &Model{
		state:            stateIdle,
		bus:              b,
		thinkingRenderer: tr,
		toolRenderer:     tlr,
		spinnerProvider:  sp,
		stream:           s,
		animator:         a,
		width:            chatWindowWidth,
		flushFn:          func(content string) tea.Cmd { return tea.Printf("%s", content) },
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Init returns the initial command (waitForEvent).
func (m *Model) Init() tea.Cmd {
	return waitForEvent(m.bus.UIUpdates())
}

// Update processes messages and drives the state machine.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyCtrlC {
		return m.handleCancel()
	}

	if _, ok := msg.(channelClosedMsg); ok {
		slog.Error("bus channel closed before DoneEvent")
		return m, tea.Quit
	}

	switch m.state {
	case stateIdle:
		return m.updateIdle(msg)
	case stateThinking:
		return m.updateThinking(msg)
	case stateStreaming:
		return m.updateStreaming(msg)
	case stateTooling:
		return m.updateTooling(msg)
	case stateDone:
		return m, tea.Quit
	}
	return m, nil
}

// View renders the current state.
func (m *Model) View() string {
	switch m.state {
	case stateIdle:
		return m.stream.Pending()
	case stateThinking:
		return m.thinkingRenderer.RenderThinking(ui.StatusRunning, m.thinkingStart, m.spinnerFrame, m.spinnerProvider)
	case stateStreaming:
		return m.stream.Pending()
	case stateTooling:
		return m.renderToolsView()
	}
	return ""
}

func (m *Model) thinkingResult() string {
	return "Thought for " + time.Since(m.thinkingStart).Round(time.Second).String()
}

func (m *Model) handleCancel() (tea.Model, tea.Cmd) {
	var flushCmd tea.Cmd
	switch m.state {
	case stateIdle:
		flushCmd = m.doFlush(m.stream.Flush())
	case stateThinking:
		flushCmd = m.doFlush([]string{m.thinkingRenderer.RenderThinking(ui.StatusError, m.thinkingStart, m.spinnerFrame, m.spinnerProvider)})
	case stateStreaming:
		m.animator.FlushAll()
		flushCmd = m.doFlush(m.stream.Flush())
	case stateTooling:
		for i := range m.tools {
			if m.tools[i].status == ui.StatusRunning {
				m.tools[i].status = ui.StatusError
			}
		}
		flushCmd = m.doFlush(m.renderAllTools())
		m.tools = nil
	}
	m.bus.SendAction(domain.StopAction{})
	if flushCmd != nil {
		return m, tea.Sequence(flushCmd, tea.Quit)
	}
	return m, tea.Quit
}

func (m *Model) updateIdle(msg tea.Msg) (tea.Model, tea.Cmd) {
	ev, ok := msg.(eventMsg)
	if !ok {
		return m, waitForEvent(m.bus.UIUpdates())
	}

	switch u := ev.update.(type) {
	case domain.ThinkingEvent:
		flushCmd := m.doFlush(m.stream.Flush())
		m.state = stateThinking
		m.thinkingStart = time.Now()
		m.spinnerFrame = 0
		return m, tea.Batch(flushCmd, waitForEvent(m.bus.UIUpdates()), animationTick())
	case domain.TextEvent:
		m.animator.Enqueue(u.Text)
		m.state = stateStreaming
		return m, tea.Batch(waitForEvent(m.bus.UIUpdates()), animationTick())
	case domain.ToolStartEvent:
		flushCmd := m.doFlush(m.stream.Flush())
		m.tools = append(m.tools, toolSlot{
			callID:   u.CallID,
			toolName: u.ToolName,
			display:  u.Display,
			status:   ui.StatusRunning,
		})
		m.state = stateTooling
		return m, tea.Batch(flushCmd, waitForEvent(m.bus.UIUpdates()), animationTick())
	case domain.DoneEvent:
		flushCmd := m.doFlush(m.stream.Flush())
		m.state = stateDone
		return m, tea.Sequence(flushCmd, tea.Quit)
	}
	return m, waitForEvent(m.bus.UIUpdates())
}
func (m *Model) updateThinking(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tickMsg); ok {
		m.spinnerFrame++
		return m, tea.Batch(waitForEvent(m.bus.UIUpdates()), animationTick())
	}

	ev, ok := msg.(eventMsg)
	if !ok {
		return m, tea.Batch(waitForEvent(m.bus.UIUpdates()), animationTick())
	}

	switch u := ev.update.(type) {
	case domain.TextEvent:
		flushCmd := m.doFlush([]string{m.thinkingRenderer.RenderThinking(ui.StatusSuccess, m.thinkingStart, m.spinnerFrame, m.spinnerProvider)})
		m.animator.Enqueue(u.Text)
		m.state = stateStreaming
		return m, tea.Batch(waitForEvent(m.bus.UIUpdates()), animationTick(), flushCmd)
	case domain.ToolStartEvent:
		flushCmd := m.doFlush([]string{m.thinkingRenderer.RenderThinking(ui.StatusSuccess, m.thinkingStart, m.spinnerFrame, m.spinnerProvider)})
		m.tools = append(m.tools, toolSlot{
			callID:   u.CallID,
			toolName: u.ToolName,
			display:  u.Display,
			status:   ui.StatusRunning,
		})
		m.state = stateTooling
		return m, tea.Batch(waitForEvent(m.bus.UIUpdates()), animationTick(), flushCmd)
	case domain.DoneEvent:
		flushCmd := m.doFlush([]string{m.thinkingRenderer.RenderThinking(ui.StatusSuccess, m.thinkingStart, m.spinnerFrame, m.spinnerProvider)})
		m.state = stateDone
		return m, tea.Sequence(flushCmd, tea.Quit)
	}
	return m, tea.Batch(waitForEvent(m.bus.UIUpdates()), animationTick())
}

func (m *Model) updateStreaming(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tickMsg:
		chunk, ok := m.animator.NextChunk()
		if !ok {
			m.state = stateIdle
			return m, waitForEvent(m.bus.UIUpdates())
		}
		blocks := m.stream.Append(chunk)
		flushCmd := m.doFlush(blocks)
		return m, tea.Batch(waitForEvent(m.bus.UIUpdates()), animationTick(), flushCmd)
	}

	ev, ok := msg.(eventMsg)
	if !ok {
		return m, tea.Batch(waitForEvent(m.bus.UIUpdates()), animationTick())
	}

	switch u := ev.update.(type) {
	case domain.TextEvent:
		m.animator.Enqueue(u.Text)
		return m, tea.Batch(waitForEvent(m.bus.UIUpdates()), animationTick())
	case domain.ThinkingEvent:
		raw := m.animator.FlushAll()
		if raw != "" {
			m.stream.Append(raw)
		}
		flushCmd := m.doFlush(m.stream.Flush())
		m.state = stateThinking
		m.thinkingStart = time.Now()
		m.spinnerFrame = 0
		return m, tea.Batch(waitForEvent(m.bus.UIUpdates()), animationTick(), flushCmd)
	case domain.ToolStartEvent:
		raw := m.animator.FlushAll()
		if raw != "" {
			m.stream.Append(raw)
		}
		flushCmd := m.doFlush(m.stream.Flush())
		m.tools = append(m.tools, toolSlot{
			callID:   u.CallID,
			toolName: u.ToolName,
			display:  u.Display,
			status:   ui.StatusRunning,
		})
		m.state = stateTooling
		return m, tea.Batch(waitForEvent(m.bus.UIUpdates()), animationTick(), flushCmd)
	case domain.DoneEvent:
		raw := m.animator.FlushAll()
		if raw != "" {
			m.stream.Append(raw)
		}
		flushCmd := m.doFlush(m.stream.Flush())
		m.state = stateDone
		return m, tea.Sequence(flushCmd, tea.Quit)
	}
	return m, tea.Batch(waitForEvent(m.bus.UIUpdates()), animationTick())
}

func (m *Model) updateTooling(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tickMsg); ok {
		m.spinnerFrame++
		return m, tea.Batch(waitForEvent(m.bus.UIUpdates()), animationTick())
	}

	ev, ok := msg.(eventMsg)
	if !ok {
		return m, tea.Batch(waitForEvent(m.bus.UIUpdates()), animationTick())
	}

	switch u := ev.update.(type) {
	case domain.ToolStartEvent:
		m.tools = append(m.tools, toolSlot{
			callID:   u.CallID,
			toolName: u.ToolName,
			display:  u.Display,
			status:   ui.StatusRunning,
		})
		return m, waitForEvent(m.bus.UIUpdates())
	case domain.ToolStreamEvent:
		for i := range m.tools {
			if m.tools[i].callID == u.CallID {
				m.tools[i].streamOutput += u.Chunk
				break
			}
		}
		return m, waitForEvent(m.bus.UIUpdates())
	case domain.ToolEndEvent:
		for i := range m.tools {
			if m.tools[i].callID == u.CallID {
				if u.Error != "" {
					m.tools[i].status = ui.StatusError
					m.tools[i].errorMsg = u.Error
				} else {
					m.tools[i].status = ui.StatusSuccess
				}
				break
			}
		}
		flushed := m.flushCompletedToolPrefix()
		flushCmd := m.doFlush(flushed)
		if len(m.tools) == 0 {
			m.state = stateIdle
			return m, tea.Batch(waitForEvent(m.bus.UIUpdates()), flushCmd)
		}
		return m, tea.Batch(waitForEvent(m.bus.UIUpdates()), animationTick(), flushCmd)
	case domain.DoneEvent:
		flushCmd := m.doFlush(m.renderAllTools())
		m.tools = nil
		m.state = stateDone
		return m, tea.Sequence(flushCmd, tea.Quit)
	}
	return m, waitForEvent(m.bus.UIUpdates())
}

// doFlush writes blocks to history and returns a batched Cmd for any flush operations.
func (m *Model) doFlush(blocks []string) tea.Cmd {
	var cmds []tea.Cmd
	for _, b := range blocks {
		if b != "" && m.flushFn != nil {
			if c := m.flushFn(b); c != nil {
				cmds = append(cmds, c)
			}
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (m *Model) drainAnimator() {
	raw := m.animator.FlushAll()
	if raw != "" {
		m.stream.Append(raw)
	}
}

func (m *Model) flushCompletedToolPrefix() []string {
	var flushed []string
	for len(m.tools) > 0 && m.tools[0].status != ui.StatusRunning {
		flushed = append(flushed, m.renderToolBox(m.tools[0]))
		m.tools = m.tools[1:]
	}
	return flushed
}

func (m *Model) renderToolBox(slot toolSlot) string {
	boxWidth := m.width - boxWidthPad
	if boxWidth < 1 {
		boxWidth = 1
	}
	prefix := m.toolRenderer.StatusPrefix(slot.status, m.spinnerProvider.Frame(m.spinnerFrame))

	var rendered string
	switch d := slot.display.(type) {
	case domain.StringDisplay:
		rendered = m.toolRenderer.RenderString(d, slot.status, slot.errorMsg, prefix)
	case domain.DiffDisplay:
		rendered = m.toolRenderer.RenderDiff(d, slot.status, slot.errorMsg, prefix)
	case domain.ShellDisplay:
		output := slot.streamOutput
		if d.CapturedOutput != nil && *d.CapturedOutput != "" {
			output = *d.CapturedOutput
		}
		rendered = m.toolRenderer.RenderShell(d, output, slot.status, slot.errorMsg, prefix)
	default:
		return ""
	}

	if rendered == "" {
		return ""
	}
	return m.toolRenderer.Box(rendered, boxWidth, slot.status)
}

func (m *Model) renderToolsView() string {
	return strings.Join(m.renderAllTools(), "")
}

func (m *Model) renderAllTools() []string {
	var out []string
	for _, slot := range m.tools {
		s := m.renderToolBox(slot)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}


// DrainAnimationForTest runs tick updates until the animator is drained.
// Used by golden tests to complete text streaming before capturing output.
func (m *Model) DrainAnimationForTest() *Model {
	for i := 0; i < 1000; i++ {
		if m.state != stateStreaming {
			break
		}
		res, _ := m.Update(tickMsg{})
		m = res.(*Model)
	}
	return m
}
