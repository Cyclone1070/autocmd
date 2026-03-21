package prompt

import (
	"strings"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
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
	HasPending() bool
	FlushAll() string
}

type bus interface {
	UIUpdates() <-chan domain.UIUpdate
	SendAction(domain.Action)
}

type viewportGater interface {
	Gate(content string) string
}

type uiState int

const (
	stateIdle uiState = iota
	stateThinking
	stateStreaming
	stateTooling
	stateFlushing
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

type Model struct {
	state   uiState
	bus     bus
	stream  stream
	animator animator
	tools   []toolSlot

	thinkingRenderer thinkingRenderer
	toolRenderer     toolRenderer
	spinnerProvider  spinnerProvider

	width         int
	gater         viewportGater
	theme         *ui.Theme
	flushFn       func(content string) tea.Cmd
	thinkingStart time.Time
	spinnerFrame  int
	nextState     uiState
	// isPolling is true while a pollBus goroutine is blocked waiting on the bus.
	isPolling bool
	// isCancelling gates late bus events once Ctrl+C has initiated terminal cancel flow.
	isCancelling bool
}

type Option func(*Model)

func WithTheme(th *ui.Theme) Option {
	return func(m *Model) {
		m.theme = th
	}
}

func WithFlush(fn func(content string) tea.Cmd) Option {
	return func(m *Model) {
		m.flushFn = fn
	}
}

func NewModel(
	b bus,
	tr thinkingRenderer,
	tlr toolRenderer,
	sp spinnerProvider,
	th *ui.Theme,
	s stream,
	a animator,
	g viewportGater,
	chatWindowWidth int,
	opts ...Option,
) *Model {
	m := &Model{
		state:            stateIdle,
		bus:              b,
		thinkingRenderer: tr,
		toolRenderer:     tlr,
		spinnerProvider:  sp,
		theme:            th,
		stream:           s,
		animator:         a,
		width:            chatWindowWidth,
		gater:            g,
		flushFn:          func(content string) tea.Cmd { return tea.Printf("%s", content) },
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (m *Model) Init() tea.Cmd {
	m.isPolling = true
	return tea.Batch(animationTick(tickHighDelay), m.pollBus())
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyCtrlC {
		return m.handleCancel()
	}

	switch msg := msg.(type) {
	case tickMsg:
		return m.handleTick()
	case busEventMsg:
		if m.isCancelling {
			return m, nil
		}
		return m.handleBusEvent(msg.event)
	case busClosedMsg:
		return m.handleUnexpectedClose()
	case flushDoneMsg:
		return m.handleFlushDone()
	case animatorDrainedMsg:
		return m.tryResumePoll()
	}

	return m, nil
}

func (m *Model) pollBus() tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-m.bus.UIUpdates()
		if !ok {
			return busClosedMsg{}
		}
		return busEventMsg{event: ev}
	}
}

func (m *Model) handleTick() (tea.Model, tea.Cmd) {
	if m.state == stateFlushing {
		return m, m.nextTick()
	}

	if m.state != stateIdle && m.state != stateDone && m.state != stateFlushing {
		m.spinnerFrame++
	}

	if m.state == stateStreaming && m.animator.HasPending() {
		chunk, ok := m.animator.NextChunk()
		if !ok {
			m.state = stateIdle
			return m, m.nextTick()
		}
		blocks := m.stream.Append(chunk)
		if len(blocks) > 0 {
			// Tick-triggered flush must still arm the next tick; handleFlushDone is poll-only.
			_, flushCmd := m.doFlush(blocks, stateStreaming)
			return m, tea.Batch(flushCmd, m.nextTick())
		}
		if !m.animator.HasPending() {
			return m, tea.Batch(m.nextTick(), signalAnimatorDrained())
		}
		return m, m.nextTick()
	}

	return m, m.nextTick()
}

func (m *Model) tryResumePoll() (tea.Model, tea.Cmd) {
	return m.withPollIfNeeded()
}

func (m *Model) withPollIfNeeded() (tea.Model, tea.Cmd) {
	if !m.isPolling && m.isReadyForEvent() {
		m.isPolling = true
		return m, m.pollBus()
	}
	return m, nil
}

func (m *Model) schedulePollOnly() (tea.Model, tea.Cmd) {
	m.isPolling = true
	return m, m.pollBus()
}

func (m *Model) isReadyForEvent() bool {
	switch m.state {
	case stateIdle, stateThinking, stateTooling:
		return true
	case stateStreaming:
		return !m.animator.HasPending()
	}
	return false
}

func (m *Model) handleUnexpectedClose() (tea.Model, tea.Cmd) {
	m.isPolling = false
	errLine := "Error: bus closed unexpectedly"
	if m.theme != nil {
		errLine = m.theme.Error(errLine)
	}
	errLine = "\n " + errLine

	switch m.state {
	case stateThinking:
		thinkingLine := m.thinkingRenderer.RenderThinking(ui.StatusError, m.thinkingStart, m.spinnerFrame, m.spinnerProvider)
		return m.doFlush([]string{thinkingLine, errLine}, stateDone)
	case stateStreaming:
		m.animator.FlushAll()
		blocks := append(m.stream.Flush(), errLine)
		return m.doFlush(blocks, stateDone)
	case stateTooling:
		for i := range m.tools {
			if m.tools[i].status == ui.StatusRunning {
				m.tools[i].status = ui.StatusError
				m.tools[i].errorMsg = "bus closed unexpectedly"
			}
		}
		return m.doFlush(m.renderAllTools(), stateDone)
	default:
		blocks := append(m.stream.Flush(), errLine)
		return m.doFlush(blocks, stateDone)
	}
}

func (m *Model) handleBusEvent(u domain.UIUpdate) (tea.Model, tea.Cmd) {
	m.isPolling = false

	var flushBlocks []string
	if m.state == stateThinking {
		flushBlocks = append(flushBlocks, m.thinkingRenderer.RenderThinking(ui.StatusSuccess, m.thinkingStart, m.spinnerFrame, m.spinnerProvider))
	}

	switch u := u.(type) {
	case domain.ThinkingEvent:
		m.state = stateThinking
		m.thinkingStart = time.Now()
		m.spinnerFrame = 0
		flushBlocks = append(flushBlocks, m.stream.Flush()...)
		return m.doFlush(flushBlocks, stateThinking)
	case domain.TextEvent:
		if u.IsThought {
			return m.schedulePollOnly()
		}
		m.animator.Enqueue(u.Text)
		m.state = stateStreaming
		if len(flushBlocks) > 0 {
			return m.doFlush(flushBlocks, stateStreaming)
		}
		return m, nil
	case domain.ToolStartEvent:
		m.tools = append(m.tools, toolSlot{
			callID:   u.CallID,
			toolName: u.ToolName,
			display:  u.Display,
			status:   ui.StatusRunning,
		})
		m.state = stateTooling
		flushBlocks = append(flushBlocks, m.stream.Flush()...)
		return m.doFlush(flushBlocks, stateTooling)
	case domain.DoneEvent:
		m.state = stateDone
		flushBlocks = append(flushBlocks, m.stream.Flush()...)
		return m.doFlush(flushBlocks, stateDone)
	case domain.ToolStreamEvent:
		if m.state == stateTooling {
			for i := range m.tools {
				if m.tools[i].callID == u.CallID {
					m.tools[i].streamOutput += u.Chunk
					break
				}
			}
		}
		return m.schedulePollOnly()
	case domain.ToolEndEvent:
		if m.state == stateTooling {
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
			if len(flushed) > 0 {
				next := stateTooling
				if len(m.tools) == 0 {
					next = stateIdle
				}
				flushed = append(flushBlocks, flushed...)
				return m.doFlush(flushed, next)
			}
			if len(m.tools) == 0 {
				m.state = stateIdle
			}
		}
		if len(flushBlocks) > 0 {
			return m.doFlush(flushBlocks, m.state)
		}
		return m.schedulePollOnly()
	}
	if len(flushBlocks) > 0 {
		return m.doFlush(flushBlocks, m.state)
	}
	return m.schedulePollOnly()
}

func (m *Model) handleFlushDone() (tea.Model, tea.Cmd) {
	m.state = m.nextState
	if m.state == stateDone || m.isCancelling {
		return m, tea.Quit
	}
	return m.withPollIfNeeded()
}

func (m *Model) doFlush(blocks []string, next uiState) (tea.Model, tea.Cmd) {
	if len(blocks) == 0 {
		m.state = next
		if m.state == stateDone {
			return m, tea.Quit
		}
		return m.withPollIfNeeded()
	}

	m.state = stateFlushing
	m.nextState = next

	var cmds []tea.Cmd
	for _, b := range blocks {
		if b != "" && m.flushFn != nil {
			cmds = append(cmds, m.flushFn(b))
		}
	}

	if len(cmds) == 0 {
		m.state = next
		if m.state == stateDone {
			return m, tea.Quit
		}
		return m.withPollIfNeeded()
	}

	cmds = append(cmds, func() tea.Msg { return flushDoneMsg{} })
	return m, tea.Sequence(cmds...)
}

func (m *Model) nextTick() tea.Cmd {
	delay := tickHighDelay
	if m.state == stateStreaming {
		delay = tickLowDelay
	}
	return animationTick(delay)
}

func (m *Model) View() string {
	var content string
	switch m.state {
	case stateIdle, stateFlushing:
		content = m.stream.Pending()
	case stateThinking:
		content = m.thinkingRenderer.RenderThinking(ui.StatusRunning, m.thinkingStart, m.spinnerFrame, m.spinnerProvider)
	case stateStreaming:
		content = m.stream.Pending()
	case stateTooling:
		content = m.renderToolsView()
	}
	return m.gater.Gate(content)
}

func (m *Model) handleCancel() (tea.Model, tea.Cmd) {
	if m.isCancelling {
		return m, nil
	}
	m.isCancelling = true
	m.bus.SendAction(domain.StopAction{})
	m.isPolling = false

	switch m.state {
	case stateThinking:
		return m.doFlush([]string{m.thinkingRenderer.RenderThinking(ui.StatusError, m.thinkingStart, m.spinnerFrame, m.spinnerProvider)}, stateDone)
	case stateStreaming:
		m.animator.FlushAll()
		return m.doFlush(m.stream.Flush(), stateDone)
	case stateTooling:
		for i := range m.tools {
			if m.tools[i].status == ui.StatusRunning {
				m.tools[i].status = ui.StatusError
				m.tools[i].errorMsg = "cancelled"
			}
		}
		return m.doFlush(m.renderAllTools(), stateDone)
	case stateFlushing:
		switch m.nextState {
		case stateThinking:
			return m.doFlush([]string{m.thinkingRenderer.RenderThinking(ui.StatusError, m.thinkingStart, m.spinnerFrame, m.spinnerProvider)}, stateDone)
		case stateStreaming:
			m.animator.FlushAll()
			return m.doFlush(m.stream.Flush(), stateDone)
		case stateTooling:
			for i := range m.tools {
				if m.tools[i].status == ui.StatusRunning {
					m.tools[i].status = ui.StatusError
					m.tools[i].errorMsg = "cancelled"
				}
			}
			return m.doFlush(m.renderAllTools(), stateDone)
		default:
			return m.doFlush(m.stream.Flush(), stateDone)
		}
	default:
		return m.doFlush(m.stream.Flush(), stateDone)
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
	boxWidth := m.width - 2
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
	return strings.Join(m.renderAllTools(), "\n")
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
