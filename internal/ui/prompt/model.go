// Package prompt provides the main interactive prompt and tool execution UI.
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
	RenderThinking(status ui.ToolStatus, start time.Time, tick int, thoughtText string, sp spinnerProvider) string
}

type toolRenderer interface {
	RenderString(d domain.StringDisplay, status ui.ToolStatus, err string, frame string) string
	RenderDiff(d domain.DiffDisplay, status ui.ToolStatus, err string, frame string) string
	RenderBash(d domain.BashDisplay, output string, status ui.ToolStatus, err string, frame string) string
	RenderQuestion(d domain.QuestionDisplay, state ui.QuestionUIState, status ui.ToolStatus, err string, frame string) string
}

type stream interface {
	Append(text string) []string
	Flush() []string
	Pending() string
	ClearBuffer()
}

type bus interface {
	UIUpdates() <-chan domain.UIUpdate
	SendAction(domain.Action)
}

type viewportGater interface {
	Gate(lines []string) ([]string, int)
}

type uiState int

const (
	stateIdle uiState = iota
	stateThinking
	stateTooling
	stateFlushing
	stateDone
)

type toolSlot struct {
	display          domain.ToolDisplay
	callID           string
	errorMsg         string
	streamOutput     string
	questionState    ui.QuestionUIState
	status           ui.ToolStatus
	awaitingApproval bool
}

// Model is the main bubbletea model for the interactive prompt.
type Model struct {
	thinkingStart    time.Time
	gater            viewportGater
	bus              bus
	stream           stream
	thinkingRenderer thinkingRenderer
	toolRenderer     toolRenderer
	spinnerProvider  spinnerProvider
	flushFn          func(content string) tea.Cmd
	theme            *ui.Theme
	thoughtText      string
	tools            []toolSlot
	state            uiState
	width            int
	spinnerFrame     int
	nextState        uiState
	isPolling        bool
	cancelRequested  bool
}

// Option is a functional option for configuring the prompt Model.
type Option func(*Model)

// WithTheme sets the visual theme for the prompt.
func WithTheme(th *ui.Theme) Option {
	return func(m *Model) {
		m.theme = th
	}
}

// WithFlush sets a custom flush function for rendering final content.
func WithFlush(fn func(content string) tea.Cmd) Option {
	return func(m *Model) {
		m.flushFn = fn
	}
}

// NewModel creates a new prompt Model with the specified dependencies.
func NewModel(
	b bus,
	tr thinkingRenderer,
	tlr toolRenderer,
	sp spinnerProvider,
	th *ui.Theme,
	s stream,
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
		width:            chatWindowWidth,
		gater:            g,
		flushFn:          func(content string) tea.Cmd { return tea.Printf("%s", content) },
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Init initializes the prompt model.
func (m *Model) Init() tea.Cmd {
	m.isPolling = true
	return tea.Batch(animationTick(tickHighDelay), m.pollBus())
}

// Update handles incoming messages from the bus and user input.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m, m.handleKey(msg)
	case tickMsg:
		return m.handleTick()
	case busEventMsg:
		if m.cancelRequested {
			switch msg.event.(type) {
			case domain.ToolEndEvent, domain.DoneEvent:
				return m.handleBusEvent(msg.event)
			default:
				// Trash queued non-terminal workflow activity after cancellation.
				// But re-arm polling so we can still receive already-buffered terminal events
				// (DoneEvent / ToolEndEvent) and avoid UI freezing.
				m.isPolling = false
				return m.withPollIfNeeded()
			}
		}
		return m.handleBusEvent(msg.event)
	case busClosedMsg:
		return m.handleUnexpectedClose()
	case flushDoneMsg:
		return m.handleFlushDone()
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

	return m, m.nextTick()
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
		thinkingLine := m.thinkingRenderer.RenderThinking(ui.StatusError, m.thinkingStart, m.spinnerFrame, "", m.spinnerProvider)
		return m.doFlush([]string{thinkingLine, errLine}, stateDone)
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
		thinkingStatus := ui.StatusSuccess
		if m.cancelRequested {
			thinkingStatus = ui.StatusError
		}
		flushBlocks = append(flushBlocks, m.thinkingRenderer.RenderThinking(thinkingStatus, m.thinkingStart, m.spinnerFrame, "", m.spinnerProvider))
	}

	switch u := u.(type) {
	case domain.ThinkingEvent:
		m.state = stateThinking
		m.thinkingStart = time.Now()
		m.thoughtText = ""
		m.spinnerFrame = 0
		flushBlocks = append(flushBlocks, m.stream.Flush()...)
		return m.doFlush(flushBlocks, stateThinking)
	case domain.TextEvent:
		if u.IsThought {
			if m.state == stateThinking {
				m.thoughtText += u.Text
			}
			return m.schedulePollOnly()
		}
		flushBlocks = append(flushBlocks, m.stream.Append(u.Text)...)
		m.state = stateIdle
		if len(flushBlocks) > 0 {
			return m.doFlush(flushBlocks, stateIdle)
		}
		return m.schedulePollOnly()
	case domain.ToolStartEvent:
		slot := toolSlot{
			callID:  u.CallID,
			display: u.Display,
			status:  ui.StatusRunning,
		}
		if qd, ok := u.Display.(domain.QuestionDisplay); ok {
			slot.questionState = ui.NewQuestionUIState(qd)
		}
		m.tools = append(m.tools, slot)
		m.state = stateTooling
		flushBlocks = append(flushBlocks, m.stream.Flush()...)
		return m.doFlush(flushBlocks, stateTooling)
	case domain.ToolApprovalRequestEvent:
		if m.state == stateTooling {
			for i := range m.tools {
				if m.tools[i].callID == u.CallID {
					m.tools[i].awaitingApproval = true
					break
				}
			}
		}
		return m.schedulePollOnly()
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
					m.tools[i].awaitingApproval = false
					if u.Display != nil {
						m.tools[i].display = u.Display
						m.tools[i].errorMsg = ""
						if u.Display.GetError() != "" {
							m.tools[i].status = ui.StatusError
						} else {
							m.tools[i].status = ui.StatusSuccess
						}
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
	if m.state == stateDone {
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

	fullContent := ui.NormalizeBlock(strings.Join(blocks, ""))
	if fullContent != "" && m.flushFn != nil {
		return m, tea.Sequence(m.flushFn(fullContent), func() tea.Msg { return flushDoneMsg{} })
	}

	m.state = next
	if m.state == stateDone {
		return m, tea.Quit
	}
	return m.withPollIfNeeded()
}

func (m *Model) nextTick() tea.Cmd {
	return animationTick(tickHighDelay)
}

// View renders the current state of the prompt.
func (m *Model) View() string {
	var content string
	switch m.state {
	case stateIdle, stateFlushing:
		content = m.stream.Pending()
	case stateThinking:
		status := ui.StatusRunning
		if m.cancelRequested {
			status = ui.StatusError
		}
		content = ui.NormalizeBlock(m.thinkingRenderer.RenderThinking(status, m.thinkingStart, m.spinnerFrame, m.thoughtText, m.spinnerProvider))
	case stateTooling:
		content = ui.NormalizeBlock(strings.Join(m.renderAllTools(), ""))
	}
	gated, _ := m.gater.Gate(strings.Split(content, "\n"))
	return strings.Join(gated, "\n")
}

func (m *Model) handleKey(key tea.KeyMsg) tea.Cmd {
	if key.Type == tea.KeyCtrlC {
		return m.handleCancelKey()
	}

	if m.state != stateTooling || len(m.tools) == 0 {
		return nil
	}

	if handled := m.handleApprovalKey(key); handled {
		return nil
	}

	return m.handleQuestionKey(key)
}

func (m *Model) handleCancelKey() tea.Cmd {
	_, cmd := m.handleCancel()
	return cmd
}

func (m *Model) handleApprovalKey(key tea.KeyMsg) bool {
	approvalSlot := m.firstAwaitingApprovalSlot()
	if approvalSlot == nil {
		return false
	}

	sendDecision := func(approved bool) {
		if m.bus == nil {
			return
		}
		m.bus.SendAction(domain.PermissionDecisionAction{CallID: approvalSlot.callID, Approved: approved})
	}

	switch key.Type {
	case tea.KeyRunes:
		if len(key.Runes) != 1 {
			return false
		}
		switch key.Runes[0] {
		case 'y', 'Y':
			sendDecision(true)
			return true
		case 'n', 'N':
			sendDecision(false)
			return true
		default:
			return false
		}
	case tea.KeyEnter:
		sendDecision(true)
		return true
	case tea.KeyEsc:
		sendDecision(false)
		return true
	default:
		return false
	}
}

func (m *Model) handleQuestionKey(key tea.KeyMsg) tea.Cmd {
	slot := &m.tools[0]
	qd, ok := slot.display.(domain.QuestionDisplay)
	if !ok || slot.status != ui.StatusRunning {
		return nil
	}

	newState, outcome := ui.HandleQuestionKey(qd, slot.questionState, key)
	slot.questionState = newState
	if outcome.Cancelled {
		return m.handleCancelKey()
	}
	if outcome.Done && m.bus != nil {
		m.bus.SendAction(domain.QuestionAnswerAction{
			CallID:  slot.callID,
			Answers: outcome.Answers,
		})
	}
	return nil
}

func (m *Model) handleCancel() (tea.Model, tea.Cmd) {
	if m.cancelRequested {
		return m, nil
	}
	m.cancelRequested = true
	m.bus.SendAction(domain.StopAction{})
	if m.stream != nil {
		blocks := m.stream.Flush()
		if len(blocks) > 0 {
			return m.doFlush(blocks, m.state)
		}
	}
	return m.withPollIfNeeded()
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
	frame := m.spinnerProvider.Frame(m.spinnerFrame)

	errorMsg := slot.errorMsg
	if de := slot.display.GetError(); de != "" {
		errorMsg = de
	}

	var rendered string
	display := slot.display
	if slot.awaitingApproval && slot.status == ui.StatusRunning {
		display = decorateApprovalPrompt(slot.display)
	}
	switch d := display.(type) {
	case domain.StringDisplay:
		rendered = m.toolRenderer.RenderString(d, slot.status, errorMsg, frame)
	case domain.DiffDisplay:
		rendered = m.toolRenderer.RenderDiff(d, slot.status, errorMsg, frame)
	case domain.BashDisplay:
		output := slot.streamOutput
		if d.CapturedOutput != "" {
			output = d.CapturedOutput
		}
		rendered = m.toolRenderer.RenderBash(d, output, slot.status, errorMsg, frame)
	case domain.QuestionDisplay:
		rendered = m.toolRenderer.RenderQuestion(d, slot.questionState, slot.status, errorMsg, frame)
	default:
		return ""
	}

	if rendered == "" {
		return ""
	}
	return rendered
}

func decorateApprovalPrompt(d domain.ToolDisplay) domain.ToolDisplay {
	switch v := d.(type) {
	case domain.StringDisplay:
		v.Error = domain.ToolErrorPermissionDenied
		return v
	case domain.DiffDisplay:
		v.Error = domain.ToolErrorPermissionDenied
		return v
	case domain.BashDisplay:
		v.Error = domain.ToolErrorPermissionDenied
		return v
	default:
		return d
	}
}

func (m *Model) firstAwaitingApprovalSlot() *toolSlot {
	for i := range m.tools {
		if m.tools[i].awaitingApproval && m.tools[i].status == ui.StatusRunning {
			return &m.tools[i]
		}
	}
	return nil
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
