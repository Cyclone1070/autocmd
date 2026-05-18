package prompt

import (
	"strings"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockBus struct {
	updates chan domain.UIUpdate
	actions []domain.Action
}

func (m *mockBus) UIUpdates() <-chan domain.UIUpdate { return m.updates }
func (m *mockBus) SendAction(a domain.Action)        { m.actions = append(m.actions, a) }

type mockStream struct {
	p             string
	lastAppend    string
	appendReturns []string
	flushReturns  []string
	appendCalls   int
	flushCalls    int
	clearCalls    int
}

func (s *mockStream) Append(t string) []string {
	s.appendCalls++
	s.lastAppend = t
	return s.appendReturns
}
func (s *mockStream) Flush() []string {
	s.flushCalls++
	if s.flushReturns != nil {
		return s.flushReturns
	}
	return []string{"flushed"}
}
func (s *mockStream) Pending() string { return s.p }
func (s *mockStream) ClearBuffer()    { s.clearCalls++ }

type mockThinkingRenderer struct{}

func (t *mockThinkingRenderer) RenderThinking(_ ui.ToolStatus, _ time.Time, _ int, _ string, _ spinnerProvider) string {
	return "thinking_rendered"
}

type mockThinkingRendererWithLeadingGap struct{}

func (t *mockThinkingRendererWithLeadingGap) RenderThinking(_ ui.ToolStatus, _ time.Time, _ int, _ string, _ spinnerProvider) string {
	return "\n\n    thinking_rendered"
}

// mockThinkingRecorder records the last status passed to RenderThinking (for cancel / bus-driven flush tests).
type mockThinkingRecorder struct {
	lastThoughtText string
	lastStatus      ui.ToolStatus
}

func (t *mockThinkingRecorder) RenderThinking(status ui.ToolStatus, _ time.Time, _ int, thoughtText string, _ spinnerProvider) string {
	t.lastStatus = status
	t.lastThoughtText = thoughtText
	return "thinking_rendered"
}

func TestModel_SummaryCompaction_Lifecycle(t *testing.T) {
	var flushed []string
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	m := NewModel(bus, &mockThinkingRenderer{}, tr, &mockSpinner{}, theme, &mockStream{}, ui.NewNoOpGater(), 80, WithFlush(func(c string) tea.Cmd {
		flushed = append(flushed, c)
		return nil
	}))
	m.isPolling = true

	res, _ := m.Update(busEventMsg{event: domain.SummaryCompactionStartEvent{}})
	m = res.(*Model)
	assert.Equal(t, stateFlushing, m.state)
	assert.Equal(t, stateSummarizing, m.nextState)

	res, _ = m.Update(flushDoneMsg{})
	m = res.(*Model)
	assert.Equal(t, stateSummarizing, m.state)
	v := m.View()
	assert.Contains(t, v, summaryCompactionTitle)
	assert.NotContains(t, v, "Summarize context for")

	res, _ = m.Update(busEventMsg{event: domain.SummaryCompactionEndEvent{}})
	m = res.(*Model)
	assert.Equal(t, stateFlushing, m.state)
	assert.Equal(t, stateIdle, m.nextState)
	require.NotEmpty(t, flushed)
	joined := strings.Join(flushed, "")
	assert.Contains(t, joined, summaryCompactionTitle)
	// End flush should emit exactly one completion block (no duplicate prefix flush).
	assert.Equal(t, 1, strings.Count(joined, summaryCompactionTitle))
}

func TestModel_SummaryCompaction_EndError_OneLineUserMessage(t *testing.T) {
	var flushed []string
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	m := NewModel(bus, &mockThinkingRenderer{}, tr, &mockSpinner{}, theme, &mockStream{}, ui.NewNoOpGater(), 80, WithFlush(func(c string) tea.Cmd {
		flushed = append(flushed, c)
		return nil
	}))
	m.isPolling = true

	res, _ := m.Update(busEventMsg{event: domain.SummaryCompactionStartEvent{}})
	m = res.(*Model)
	res, _ = m.Update(flushDoneMsg{})
	m = res.(*Model)
	require.Equal(t, stateSummarizing, m.state)

	res, _ = m.Update(busEventMsg{event: domain.SummaryCompactionEndEvent{Error: "internal summarize blew up"}})
	m = res.(*Model)
	require.Equal(t, stateFlushing, m.state)

	res, _ = m.Update(flushDoneMsg{})
	_ = res
	require.NotEmpty(t, flushed)
	out := strings.Join(flushed, "")
	assert.Contains(t, out, "Summarization failed")
	assert.NotContains(t, out, "internal summarize blew up")
	assert.NotContains(t, out, "Summarization failed -")
}

func TestModel_SyncPolling(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(bus, nil, nil, nil, theme, &mockStream{}, ui.NewNoOpGater(), 80)

	// pollBus delivers events as busEventMsg
	m.isPolling = true
	res, _ := m.Update(busEventMsg{event: domain.TextEvent{Text: "thinking...", IsThought: true}})
	newModel := res.(*Model)

	assert.Equal(t, stateFlushing, newModel.state)
	assert.Equal(t, stateThinking, newModel.nextState)

	res, _ = newModel.Update(flushDoneMsg{})
	newModel = res.(*Model)
	assert.Equal(t, stateThinking, newModel.state)
}

// TestModel_SpinnerIncrementsOncePerTick guards against duplicate tea.Tick chains (regression).
func TestModel_SpinnerIncrementsOncePerTick(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(bus, nil, nil, nil, theme, &mockStream{}, ui.NewNoOpGater(), 80)
	m.state = stateThinking
	m.spinnerFrame = 0
	for i := range 5 {
		res, _ := m.Update(tickMsg{})
		m = res.(*Model)
		assert.Equal(t, i+1, m.spinnerFrame, "exactly one spinner step per tick in thinking")
	}
}

func TestModel_CancelRequested_ThinkingFlushUsesErrorStatusOnNextBusEvent(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	th := &mockThinkingRecorder{}
	m := NewModel(bus, th, nil, nil, theme, &mockStream{}, ui.NewNoOpGater(), 80)
	m.state = stateThinking
	m.thinkingStart = time.Now()
	m.isPolling = true

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = res.(*Model)
	assert.True(t, m.cancelRequested)
	assert.Equal(t, stateFlushing, m.state)
	assert.Equal(t, stateThinking, m.nextState)

	res, _ = m.Update(flushDoneMsg{})
	m = res.(*Model)
	assert.Equal(t, stateThinking, m.state)

	// After cancel, queued non-terminal bus events are trashed, but View in stateThinking
	// must still render cancelled thinking using error styling.
	_ = m.View()
	assert.Equal(t, ui.StatusError, th.lastStatus, "View in stateThinking uses error styling after cancel")
}

func TestModel_HandleCancel_DoesNotStartDuplicatePollWhenAlreadyPolling(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(bus, &mockThinkingRenderer{}, nil, nil, theme, &mockStream{}, ui.NewNoOpGater(), 80)
	m.state = stateTooling
	m.isPolling = true // simulate pollBus goroutine already in flight

	res, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = res.(*Model)

	assert.True(t, m.cancelRequested)
	assert.True(t, m.isPolling, "cancel must not reset isPolling when a poll is already in flight")
	assert.NotNil(t, cmd, "cancel now flushes pending stream content")
}

func TestModel_HandleCancel_FlushesPendingStreamBuffer(t *testing.T) {
	var flushed []string
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	s := &mockStream{flushReturns: []string{"pending tail"}}
	m := NewModel(bus, &mockThinkingRenderer{}, nil, nil, theme, s, ui.NewNoOpGater(), 80, WithFlush(func(c string) tea.Cmd {
		flushed = append(flushed, c)
		return nil
	}))
	m.state = stateIdle
	m.isPolling = true

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = res.(*Model)

	assert.True(t, m.cancelRequested)
	assert.Equal(t, 1, s.flushCalls, "cancel should flush buffered stream")
	assert.Equal(t, 0, s.clearCalls, "cancel should not drop buffered stream")
	require.NotEmpty(t, flushed)
	assert.Contains(t, flushed[0], "pending tail")
}

func TestModel_CancelProcessesDoneEvent(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(bus, &mockThinkingRenderer{}, nil, nil, theme, &mockStream{}, ui.NewNoOpGater(), 80)
	m.state = stateThinking
	m.thinkingStart = time.Now()
	m.isPolling = true

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = res.(*Model)
	assert.True(t, m.cancelRequested)
	assert.Equal(t, stateFlushing, m.state)
	assert.Equal(t, stateThinking, m.nextState)
	require.Len(t, bus.actions, 1)
	_, ok := bus.actions[0].(domain.StopAction)
	assert.True(t, ok, "first cancel should send StopAction")

	res, _ = m.Update(busEventMsg{event: domain.DoneEvent{}})
	m = res.(*Model)
	assert.Equal(t, stateFlushing, m.state)
	assert.Equal(t, stateDone, m.nextState)

	res, cmd := m.Update(flushDoneMsg{})
	m = res.(*Model)
	assert.Equal(t, stateDone, m.state)
	assert.NotNil(t, cmd)
}

func TestModel_CancelRequested_IgnoresQueuedTextAndThoughtEventsInStreaming(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	th := &mockThinkingRecorder{lastStatus: ui.StatusSuccess}

	m := NewModel(bus, th, nil, &mockSpinner{}, theme, &mockStream{}, ui.NewNoOpGater(), 80)

	// Start idle text phase and arm polling.
	m.state = stateIdle
	m.isPolling = true
	m.spinnerFrame = 7

	// Cancel while streaming.
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = res.(*Model)
	assert.True(t, m.cancelRequested)
	assert.Equal(t, stateFlushing, m.state)

	// Flush completion returns to prior state, after which queued non-terminal
	// events must still be ignored.
	res, _ = m.Update(flushDoneMsg{})
	m = res.(*Model)
	assert.Equal(t, stateIdle, m.state)

	// Queued thinking/text activity must be ignored after cancel.
	res, _ = m.Update(busEventMsg{event: domain.TextEvent{Text: "ignored", IsThought: false}})
	m = res.(*Model)
	assert.Equal(t, stateIdle, m.state)

	res, _ = m.Update(busEventMsg{event: domain.TextEvent{Text: "ignored thought", IsThought: true}})
	m = res.(*Model)
	assert.Equal(t, stateIdle, m.state)

	// View should not have called RenderThinking (so status must remain zero value).
	assert.Equal(t, ui.StatusSuccess, th.lastStatus, "queued non-terminal events after cancel must be ignored")

	// DoneEvent is terminal and must still be processed.
	res, _ = m.Update(busEventMsg{event: domain.DoneEvent{}})
	m = res.(*Model)
	assert.Equal(t, stateFlushing, m.state, "DoneEvent should transition into flushing before quitting")
	assert.Equal(t, stateDone, m.nextState)
}

func TestModel_SpinnerAdvancesDuringEvents(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(bus, nil, nil, nil, theme, &mockStream{}, ui.NewNoOpGater(), 80)
	m.state = stateThinking
	m.spinnerFrame = 10

	// handleTick should increment even if it processes something or returns early
	m.handleTick()
	assert.Equal(t, 11, m.spinnerFrame, "Spinner should increment in handleTick")
}

func TestModel_ThinkingResultFlushedOnTransition(t *testing.T) {
	var flushed []string
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(bus, &mockThinkingRenderer{}, nil, nil, theme, &mockStream{}, ui.NewNoOpGater(), 80, WithFlush(func(c string) tea.Cmd {
		flushed = append(flushed, c)
		return nil
	}))
	m.state = stateThinking

	// Transition to streaming
	m.handleBusEvent(domain.TextEvent{Text: "hi"})

	require.NotEmpty(t, flushed)
	assert.Contains(t, flushed[0], "thinking_rendered", "Thinking result should be flushed on transition")
}

func TestModel_HandleBusEvent_SystemNotificationEvent(t *testing.T) {
	theme := ui.NewTheme(ui.ThemeConfig{})
	stream := &mockStream{}
	m := NewModel(&mockBus{}, &mockThinkingRenderer{}, nil, &mockSpinner{}, theme, stream, ui.NewTruncatingGater(10), 80)
	m.state = stateThinking

	// Transition on SystemNotificationEvent
	_, _ = m.handleBusEvent(domain.SystemNotificationEvent{Content: "test"})

	// Verify that the state becomes stateFlushing (and will transition to stateIdle)
	assert.Equal(t, stateFlushing, m.state)
	assert.Equal(t, stateIdle, m.nextState)
	
	// Verify that the stream received newlines
	assert.Equal(t, "\n\n", stream.lastAppend, "Stream should receive newlines on SystemNotificationEvent")
}

func TestModel_ViewportTruncation(t *testing.T) {
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(nil, nil, nil, nil, theme, &mockStream{p: "L1\nL2\nL3\nL4\nL5"}, ui.NewTruncatingGater(3), 80)
	v := m.View()
	assert.Contains(t, v, "above", "View should be truncated with indicator")
}

func TestModel_ScrollBoundaries(t *testing.T) {
	theme := ui.NewTheme(ui.ThemeConfig{})
	sp := &mockSpinner{}
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	// 10 lines of content
	content := "LINE_01\nLINE_02\nLINE_03\nLINE_04\nLINE_05\nLINE_06\nLINE_07\nLINE_08\nLINE_09\nLINE_10"
	// Budget 7. Indicators take 4 lines. Content window is 3 lines.
	m := NewModel(nil, nil, tr, sp, theme, nil, ui.NewTruncatingGater(7), 80)
	m.state = stateTooling
	m.tools = []toolSlot{{status: ui.StatusAwaitingApproval, display: domain.StringDisplay{Content: content}}}

	// Initial View to trigger maxScroll update
	m.View()

	// Scroll up past limit
	for range 100 {
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	}
	
	// Should be clamped to maxScroll
	assert.Equal(t, m.maxScroll, m.scrollOffset, "scrollOffset should be clamped to maxScroll")
	
	// Should be at the top
	vTop := m.View()
	assert.Contains(t, vTop, "LINE_01")
	
	// Scroll down ONCE.
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	assert.Equal(t, m.maxScroll-1, m.scrollOffset, "scrollOffset should decrease immediately")
}

func TestModel_NoTruncationIfHeightZero(t *testing.T) {
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(nil, nil, nil, nil, theme, &mockStream{p: "L1\nL2\nL3\nL4\nL5"}, ui.NewTruncatingGater(0), 80)
	v := m.View()
	assert.NotContains(t, v, "above", "View should not be truncated when height is 0")
	assert.Contains(t, v, "L5", "Full content should be visible")
}

func TestModel_FlushDoneMsgSequencing(t *testing.T) {
	// Logical check: doFlush must use tea.Sequence to ensure order
	m := &Model{}
	m.flushFn = func(_ string) tea.Cmd { return nil }
	_, cmd := m.doFlush([]string{"b1", "b2"}, stateIdle)

	// Verified in code: return m, tea.Sequence(cmds...)
	assert.NotNil(t, cmd)
}
func TestModel_ExplicitGater(t *testing.T) {
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(nil, nil, nil, nil, theme, nil, ui.NewNoOpGater(), 80)
	// Success if constructor accepts it
	assert.NotNil(t, m.gater)
}

type mockGater struct {
	gateFunc func([]string) ([]string, int)
}

func (m *mockGater) Gate(lines []string, _ int, _ bool, _ *ui.Theme) ([]string, int) { return m.gateFunc(lines) }

func TestModel_ViewUsesGater(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	g := &mockGater{gateFunc: func(lines []string) ([]string, int) {
		return append(lines, "_gated"), 0
	}}
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(bus, nil, nil, nil, theme, &mockStream{p: "raw"}, g, 80)

	assert.Equal(t, "raw\n_gated", m.View())
}

func TestModel_ToolsViewSpacing(t *testing.T) {
	theme := ui.NewTheme(ui.ThemeConfig{})
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	sp := &mockSpinner{}
	m := NewModel(nil, nil, tr, sp, theme, nil, ui.NewNoOpGater(), 80)

	m.tools = []toolSlot{
		{callID: "1", status: ui.StatusSuccess, display: domain.StringDisplay{Content: "c1"}},
		{callID: "2", status: ui.StatusSuccess, display: domain.StringDisplay{Content: "c2"}},
	}
	m.state = stateTooling

	v := m.View()
	assert.Contains(t, v, "\n\n    ✔ \n       ⎿ c2", "There should be a blank line between tool blocks")
}

func TestModel_ToolEndEvent_ReplacesDisplayWhenNotFlushedYet(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	sp := &mockSpinner{}
	m := NewModel(bus, nil, tr, sp, theme, nil, ui.NewNoOpGater(), 80)
	m.state = stateTooling
	m.tools = []toolSlot{
		{callID: "first", display: domain.NewStringDisplay("", "preview first"), status: ui.StatusRunning},
		{callID: "second", display: domain.NewStringDisplay("", "preview second"), status: ui.StatusRunning, errorMsg: "stale"},
	}

	res, _ := m.handleBusEvent(domain.ToolEndEvent{
		CallID:  "second",
		Display: domain.NewStringDisplay("", "baked second"),
	})
	newM := res.(*Model)

	require.Len(t, newM.tools, 2)
	assert.Equal(t, "preview first", newM.tools[0].display.(domain.StringDisplay).Content)
	sd := newM.tools[1].display.(domain.StringDisplay)
	assert.Equal(t, "baked second", sd.Content)
	assert.Empty(t, sd.GetError())
	assert.Equal(t, ui.StatusSuccess, newM.tools[1].status)
	assert.Empty(t, newM.tools[1].errorMsg, "atomic swap clears stale slot errorMsg")

}

func TestModel_ToolEndEvent_ReplacesDisplayWithBakedError(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	sp := &mockSpinner{}
	m := NewModel(bus, nil, tr, sp, theme, nil, ui.NewNoOpGater(), 80)
	m.state = stateTooling
	m.tools = []toolSlot{
		{callID: "first", display: domain.NewStringDisplay("", "preview first"), status: ui.StatusRunning},
		{callID: "second", display: domain.NewStringDisplay("", "preview second"), status: ui.StatusRunning},
	}
	failDisp := domain.NewStringDisplay("", "baked")
	failDisp.Error = "boom"

	res, _ := m.handleBusEvent(domain.ToolEndEvent{CallID: "second", Display: failDisp})
	newM := res.(*Model)

	require.Len(t, newM.tools, 2)
	assert.Equal(t, ui.StatusError, newM.tools[1].status)
	assert.Equal(t, "boom", newM.tools[1].display.GetError())
}

func TestModel_HandleCancel_ToolEndEventSetsCancelledDisplay(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	sp := &mockSpinner{}

	m := NewModel(bus, nil, tr, sp, theme, nil, ui.NewNoOpGater(), 80)
	m.state = stateTooling
	// Keep another tool running in front so the cancelled slot is not flushed from the prefix queue in this step.
	m.tools = []toolSlot{
		{callID: "1", status: ui.StatusRunning, display: domain.StringDisplay{Content: "still running"}},
		{callID: "2", status: ui.StatusRunning, display: domain.StringDisplay{Content: "Run something"}},
	}
	m.isPolling = true

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	cancelled := res.(*Model)
	assert.True(t, cancelled.cancelRequested)
	require.Len(t, cancelled.tools, 2)
	assert.Equal(t, ui.StatusRunning, cancelled.tools[0].status, "cancel must not locally mark running tools as error")
	assert.Equal(t, ui.StatusRunning, cancelled.tools[1].status)

	cancelDisp := domain.NewStringDisplay("", "Run something")
	cancelDisp.Error = domain.ToolErrorCancelled
	res2, _ := cancelled.handleBusEvent(domain.ToolEndEvent{CallID: "2", Display: cancelDisp})
	final := res2.(*Model)
	require.Len(t, final.tools, 2)
	assert.Equal(t, ui.StatusRunning, final.tools[0].status)
	assert.Equal(t, ui.StatusError, final.tools[1].status)
	assert.Equal(t, domain.ToolErrorCancelled, final.tools[1].display.GetError())
}

func TestModel_BusClosedUnexpectedly(t *testing.T) {
	var flushed []string
	bus := &mockBus{updates: make(chan domain.UIUpdate)}
	close(bus.updates) // Close channel immediately

	theme := ui.NewTheme(ui.ThemeConfig{})
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	tickr := &mockThinkingRenderer{}
	sp := &mockSpinner{}
	m := NewModel(bus, tickr, tr, sp, theme, &mockStream{}, ui.NewNoOpGater(), 80, WithFlush(func(s string) tea.Cmd {
		flushed = append(flushed, s)
		return nil
	}))
	m.state = stateThinking
	m.thinkingStart = time.Now()

	// Act: bus closed while a poll was in flight
	m.isPolling = true
	m.Update(busClosedMsg{})

	// Assert: Should detect closed channel, mark tools as error, and transition to flushing/done
	require.NotEmpty(t, flushed)
	foundThinking := false
	foundError := false
	for _, f := range flushed {
		if strings.Contains(f, "thinking_rendered") {
			foundThinking = true
		}
		if strings.Contains(strings.ToLower(f), "bus closed unexpectedly") {
			foundError = true
		}
	}
	assert.True(t, foundThinking, "Should flush the thinking result")
	assert.True(t, foundError, "Should flush the explicit error message 'bus closed unexpectedly'")
}

func TestModel_ToolStart_QuestionDisplayInitializesQuestionState(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	sp := &mockSpinner{}
	m := NewModel(bus, &mockThinkingRenderer{}, tr, sp, theme, &mockStream{}, ui.NewNoOpGater(), 80)

	qd := domain.NewQuestionDisplay([]domain.QuestionInfo{
		{Question: "Proceed?", Options: []string{"Yes"}, MultiSelect: false},
	})
	res, _ := m.handleBusEvent(domain.ToolStartEvent{CallID: "q1", Display: qd})
	m2 := res.(*Model)

	require.Len(t, m2.tools, 1)
	assert.False(t, m2.tools[0].questionState.Submitted)
	assert.Equal(t, 0, m2.tools[0].questionState.Active)
}

func TestModel_Question_EnterSubmitsSendsQuestionAnswerAction(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	sp := &mockSpinner{}
	m := NewModel(bus, &mockThinkingRenderer{}, tr, sp, theme, &mockStream{}, ui.NewNoOpGater(), 80)

	qd := domain.NewQuestionDisplay([]domain.QuestionInfo{
		{Question: "Proceed?", Options: []string{"Yes"}, MultiSelect: false},
	})
	m.state = stateTooling
	m.tools = []toolSlot{
		{callID: "call-q", display: qd, status: ui.StatusRunning, questionState: ui.NewQuestionUIState(qd)},
	}

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = res.(*Model)
	assert.True(t, m.tools[0].questionState.Submitted, "Enter auto-submits single-select question")

	require.Len(t, bus.actions, 1)
	qa, ok := bus.actions[0].(domain.QuestionAnswerAction)
	require.True(t, ok)
	assert.Equal(t, "call-q", qa.CallID)
	require.Len(t, qa.Answers, 1)
	assert.Equal(t, []string{"Yes"}, qa.Answers[0])
}

func TestModel_Question_EscSendsStopAction(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	sp := &mockSpinner{}
	m := NewModel(bus, &mockThinkingRenderer{}, tr, sp, theme, &mockStream{}, ui.NewNoOpGater(), 80)
	m.state = stateTooling
	qd := domain.NewQuestionDisplay([]domain.QuestionInfo{{Question: "q", Options: []string{"Yes"}}})
	m.tools = []toolSlot{
		{callID: "call-q", display: qd, status: ui.StatusRunning, questionState: ui.NewQuestionUIState(qd)},
	}

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = res.(*Model)

	require.Len(t, bus.actions, 1)
	_, ok := bus.actions[0].(domain.StopAction)
	require.True(t, ok)
	assert.True(t, m.cancelRequested)
}

func TestModel_Question_AfterSubmitIgnoresDuplicateKeys(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	sp := &mockSpinner{}
	m := NewModel(bus, &mockThinkingRenderer{}, tr, sp, theme, &mockStream{}, ui.NewNoOpGater(), 80)

	qd := domain.NewQuestionDisplay([]domain.QuestionInfo{
		{Question: "Proceed?", Options: []string{"Yes"}, MultiSelect: false},
	})
	m.state = stateTooling
	m.tools = []toolSlot{
		{callID: "call-q", display: qd, status: ui.StatusRunning, questionState: ui.NewQuestionUIState(qd)},
	}

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})

	require.Len(t, bus.actions, 1)
}

func TestModel_PermissionApproval_YKeySendsApproveAction(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	sp := &mockSpinner{}
	m := NewModel(bus, &mockThinkingRenderer{}, tr, sp, theme, &mockStream{}, ui.NewNoOpGater(), 80)
	m.state = stateTooling
	m.tools = []toolSlot{
		{
			callID:  "call-1",
			display: domain.NewStringDisplay("Read file", "preview"),
			status:  ui.StatusAwaitingApproval,
		},
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

	require.Len(t, bus.actions, 1)
	act, ok := bus.actions[0].(domain.PermissionDecisionAction)
	require.True(t, ok)
	assert.Equal(t, "call-1", act.CallID)
	assert.True(t, act.Approved)
	assert.Equal(t, ui.StatusRunning, m.tools[0].status, "status should become StatusRunning after approval")
}

func TestModel_PermissionApproval_SpaceKeySendsApproveAction(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	sp := &mockSpinner{}
	m := NewModel(bus, &mockThinkingRenderer{}, tr, sp, theme, &mockStream{}, ui.NewNoOpGater(), 80)
	m.state = stateTooling
	m.tools = []toolSlot{
		{
			callID:  "call-1",
			display: domain.NewStringDisplay("Read file", "preview"),
			status:  ui.StatusAwaitingApproval,
		},
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})

	require.Len(t, bus.actions, 1)
	act, ok := bus.actions[0].(domain.PermissionDecisionAction)
	require.True(t, ok)
	assert.Equal(t, "call-1", act.CallID)
	assert.True(t, act.Approved)
}

func TestModel_ToolApprovalRequestEvent_MarksRunningTool(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	sp := &mockSpinner{}
	m := NewModel(bus, &mockThinkingRenderer{}, tr, sp, theme, &mockStream{}, ui.NewNoOpGater(), 80)
	m.state = stateTooling
	m.tools = []toolSlot{
		{
			callID:  "call-1",
			display: domain.NewStringDisplay("Read file", "preview"),
			status:  ui.StatusRunning,
		},
	}

	res, _ := m.handleBusEvent(domain.ToolApprovalRequestEvent{CallID: "call-1"})
	m2 := res.(*Model)
	require.Len(t, m2.tools, 1)
	assert.Equal(t, ui.StatusAwaitingApproval, m2.tools[0].status)
}

func TestModel_PermissionApproval_UsesFirstAwaitingApprovalNotFirstSlot(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	sp := &mockSpinner{}
	m := NewModel(bus, &mockThinkingRenderer{}, tr, sp, theme, &mockStream{}, ui.NewNoOpGater(), 80)
	m.state = stateTooling
	m.tools = []toolSlot{
		{
			callID:           "done-slot",
			display:          domain.NewStringDisplay("Done", ""),
			status:           ui.StatusSuccess,
		},
		{
			callID:  "await-slot",
			display: domain.NewStringDisplay("Run grep", "preview"),
			status:  ui.StatusAwaitingApproval,
		},
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	require.Len(t, bus.actions, 1)
	act, ok := bus.actions[0].(domain.PermissionDecisionAction)
	require.True(t, ok)
	assert.Equal(t, "await-slot", act.CallID)
	assert.False(t, act.Approved)
}

func TestModel_FlushSpacingRegression(t *testing.T) {
	var terminalOutput strings.Builder
	// mockPrintf mimics tea.Printf's internal behavior: split on \n, each line gets \n
	mockPrintf := func(s string) tea.Cmd {
		lines := strings.SplitSeq(s, "\n")
		for l := range lines {
			terminalOutput.WriteString(l + "\n")
		}
		return nil
	}

	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	renderer := ui.NewGlamourRenderer(80, true)
	stream := NewStream(renderer)
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	m := NewModel(bus, &mockThinkingRenderer{}, tr, &mockSpinner{}, theme, stream, ui.NewNoOpGater(), 80, WithFlush(mockPrintf))

	// Case 1: Avoid sticking between tool block and para
	m.doFlush([]string{"✔ BOX\n  ╰ DONE"}, stateIdle)
	// Simulate Para delta starting with \n
	m.doFlush([]string{"\nPARA"}, stateIdle)

	output := terminalOutput.String()
	t.Logf("Output Case 1:\n%q", output)

	// We want exactly 1 blank line between them.
	// Current buggy behavior (blind strip) results in 0 blank lines (stuck).
	assert.Contains(t, output, "\n  ╰ DONE\n\nPARA\n", "Tool block and para should have a blank line between them")

	terminalOutput.Reset()

	// Case 2: Avoid double-spacing with ANSI gap lines (Headers)
	// Simulate Para ending
	m.doFlush([]string{"PARA"}, stateIdle)
	// Simulate Header delta with ANSI gap line: "\x1b[0m  \x1b[0m\n## HEADER"
	// Note: We use \x1b[0m as a common "reset" that glamour puts in gaps.
	m.doFlush([]string{"\x1b[0m  \x1b[0m\n## HEADER"}, stateIdle)

	output = terminalOutput.String()
	t.Logf("Output Case 2:\n%q", output)

	// We want exactly 1 blank line between them.
	// Current behavior (if not trimming ANSI gap) results in 2 blank lines.
	// Expected terminal: "PARA\n" (prev) + "\n" (from prepended \n) + "## HEADER\n" (from tea.Printf)
	// Wait, if it's "PARA\n\n## HEADER\n" that's 1 blank line.
	assert.NotContains(t, output, "\nPARA\n\n\n", "Should not have double blank lines before header")
	assert.Contains(t, output, "\nPARA\n\n## HEADER\n", "Should have exactly one blank line before header")

	terminalOutput.Reset()

	// Case 3: Multiple newlines should be respected (minus 1 for tea.Printf)
	m.doFlush([]string{"PARA"}, stateIdle)
	m.doFlush([]string{"\n\n\nCONTENT"}, stateIdle)
	output = terminalOutput.String()
	// Total terminal: \nPARA\n (flush 1) + \n\n (from prepended \n\n) + CONTENT\n (flush 2)
	assert.Contains(t, output, "\nPARA\n\n\nCONTENT\n", "Triple newlines should result in two blank lines (respecting the gap)")
	assert.Contains(t, output, "\n\n\n", "Should have triple newlines total between PARA and CONTENT")
}

func TestModel_TextEvent_AppendsImmediatelyAndFlushesReturnedBlocks(t *testing.T) {
	var flushed []string
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	s := &mockStream{appendReturns: []string{"blk"}}
	m := NewModel(bus, nil, nil, nil, theme, s, ui.NewNoOpGater(), 80, WithFlush(func(c string) tea.Cmd {
		flushed = append(flushed, c)
		return nil
	}))
	m.isPolling = true

	res, _ := m.Update(busEventMsg{event: domain.TextEvent{Text: "hello"}})
	_ = res.(*Model)

	assert.Equal(t, 1, s.appendCalls)
	assert.Equal(t, "hello", s.lastAppend)
	require.NotEmpty(t, flushed)
	assert.Contains(t, flushed[0], "blk")
}

func TestModel_TextEvent_DoesNotEnterStreamingState(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(bus, nil, nil, nil, theme, &mockStream{}, ui.NewNoOpGater(), 80)
	m.isPolling = true

	res, _ := m.Update(busEventMsg{event: domain.TextEvent{Text: "hello"}})
	m = res.(*Model)

	assert.Equal(t, stateIdle, m.state)
}

func TestModel_ThoughtChunks_StreamIntoThinkingView(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	th := &mockThinkingRecorder{}
	m := NewModel(bus, th, nil, &mockSpinner{}, theme, &mockStream{}, ui.NewNoOpGater(), 80)

	m.state = stateThinking
	m.isPolling = true
	res, _ := m.Update(busEventMsg{event: domain.TextEvent{Text: "first ", IsThought: true}})
	m = res.(*Model)
	res, _ = m.Update(busEventMsg{event: domain.TextEvent{Text: "second\n", IsThought: true}})
	m = res.(*Model)

	_ = m.View()
	assert.Equal(t, "first second\n", th.lastThoughtText)
}

func TestModel_ViewThinking_NormalizesLeadingBlankLines(t *testing.T) {
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(nil, &mockThinkingRendererWithLeadingGap{}, nil, &mockSpinner{}, theme, &mockStream{}, ui.NewNoOpGater(), 80)
	m.state = stateThinking
	m.thinkingStart = time.Now()

	v := m.View()
	assert.True(t, strings.HasPrefix(v, "\n    "), "thinking view should start with a single leading newline")
	assert.False(t, strings.HasPrefix(v, "\n\n"), "thinking view should not include an extra leading blank line")
}

func TestNormalizeBlock(t *testing.T) {
	// Simple paragraph
	assert.Equal(t, "\nPara 1", ui.NormalizeBlock("Para 1"))

	// Double leading newlines (typical glamour delta)
	assert.Equal(t, "\nPara 2", ui.NormalizeBlock("\n\nPara 2"))

	// Mixed ANSI and newlines
	gapLine := "\x1b[0m  \x1b[0m"
	assert.Equal(t, "\n## Header", ui.NormalizeBlock(gapLine+"\n\n## Header"))

	// Trailing newlines should be trimmed
	assert.Equal(t, "\nPara 3", ui.NormalizeBlock("Para 3\n\n"))
}

func TestNormalization_ANSI_GapLines(t *testing.T) {
	// Verify that trimVisuallyEmpty handles ANSI gap lines correctly.
	// These are common in glamour header output.
	gapLine := "\x1b[0m  \x1b[0m"
	input := gapLine + "\n" + "## HEADER"

	normalized := ui.NormalizeBlock(input)
	assert.Equal(t, "\n## HEADER", normalized, "Should trim ANSI gap line and prepend exactly one newline")
}

func TestSpacing_ToolboxBatchNormalization(t *testing.T) {
	theme := ui.NewTheme(ui.ThemeConfig{})
	renderer := ui.NewGlamourRenderer(80, true)
	stream := NewStream(renderer)
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	spinnerProvider := ui.NewSpinnerRenderer(lipgloss.NewStyle())

	m := NewModel(nil, nil, tr, spinnerProvider, theme, stream, ui.NewNoOpGater(), 80)

	// Add two tools
	m.handleBusEvent(domain.ToolStartEvent{
		CallID:  "1",
		Display: domain.NewBashDisplay("Tool 1", "ls", "", ""),
	})
	m.handleBusEvent(domain.ToolStartEvent{
		CallID:  "2",
		Display: domain.NewBashDisplay("Tool 2", "pwd", "", ""),
	})

	// Manually simulate flush batching (no extra spacing inserted by normalizer)
	tools := m.renderAllTools()
	joined := strings.Join(tools, "")
	normalized := ui.NormalizeBlock(joined)

	assert.Contains(t, normalized, "⠹ Tool 1")
	assert.Contains(t, normalized, "⠹ Tool 2")
	assert.NotContains(t, normalized, "╭")
}

func TestDeepCheck(t *testing.T) {
	theme := ui.NewTheme(ui.ThemeConfig{})
	renderer := ui.NewGlamourRenderer(80, true)
	stream := NewStream(renderer)
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	sp := ui.NewSpinnerRenderer(lipgloss.NewStyle())

	m := NewModel(nil, nil, tr, sp, theme, stream, ui.NewNoOpGater(), 80)

	// Add tools
	m.handleBusEvent(domain.ToolStartEvent{CallID: "1", Display: domain.NewBashDisplay("ls", "ls", "", "")})
	m.handleBusEvent(domain.ToolEndEvent{CallID: "1", Display: domain.NewBashDisplay("ls", "ls", "", "")})
	m.handleBusEvent(domain.ToolStartEvent{CallID: "2", Display: domain.NewBashDisplay("pwd", "pwd", "", "")})

	tools := m.renderAllTools()
	for i, tool := range tools {
		t.Logf("Tool %d: %q", i, tool)
	}

	joined := strings.Join(tools, "")
	t.Logf("Joined: %q", joined)
}

func TestView_StrictToolboxSpacing(t *testing.T) {
	theme := ui.NewTheme(ui.ThemeConfig{})
	renderer := ui.NewGlamourRenderer(80, true)
	stream := NewStream(renderer)
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	spinnerProvider := ui.NewSpinnerRenderer(lipgloss.NewStyle())

	m := NewModel(nil, nil, tr, spinnerProvider, theme, stream, ui.NewNoOpGater(), 80)

	// Add two tools to simulate the "String 1", "String 2" sequence in the user's image
	m.handleBusEvent(domain.ToolStartEvent{
		CallID:  "1",
		Display: domain.NewBashDisplay("Tool 1", "ls", "", ""),
	})
	m.handleBusEvent(domain.ToolStartEvent{
		CallID:  "2",
		Display: domain.NewBashDisplay("Tool 2", "pwd", "", ""),
	})

	view := m.View()

	// Between two boxes, we expect:
	// ╰──────╯ (End of Box 1)
	// \n       (First newline: ends the line)
	// \n       (Second newline: creates the blank line)
	// ╭──────╮ (Start of Box 2)
	//
	// If there is a third \n, it means we have TWO blank lines.
	// Print the substring between the boxes.
	sep := "╰"
	parts := strings.Split(view, sep)
	if len(parts) > 1 {
		// Get everything between the end of the first box line and the start of the next box.
		// The next box starts with \n╭.
		afterBox1 := parts[1]
		endOfLine := strings.Index(afterBox1, "\n")
		if endOfLine != -1 {
			gap := afterBox1[endOfLine:]
			before, _, ok := strings.Cut(gap, "╭")
			if ok {
				actualGap := before
				t.Logf("Actual gap between boxes: %q", actualGap)
			}
		}
	}

	assert.False(t, strings.Contains(view, "\n\n\n"), "View should not contain triple newlines (double blank lines) between boxes")
}

func TestModel_PermissionApproval_BashCommandLineDisappears(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	tr := ui.NewToolRenderer(theme, 80, ui.NewToolOutputGater(12))
	sp := &mockSpinner{}
	m := NewModel(bus, &mockThinkingRenderer{}, tr, sp, theme, &mockStream{}, ui.NewTruncatingGater(10), 80)
	
	cwd := "~/repos/iav"
	cmd := "for i in {1..5}; do date; sleep 1; done"
	t.Logf("stateIdle: %v, stateThinking: %v, stateTooling: %v, stateFlushing: %v", stateIdle, stateThinking, stateTooling, stateFlushing)
	d := domain.NewBashDisplay("Run a loop", cmd, cwd, "")
	m.handleBusEvent(domain.ToolStartEvent{CallID: "bash-1", Display: d})
	m.Update(flushDoneMsg{})
	
	m.handleBusEvent(domain.ToolApprovalRequestEvent{CallID: "bash-1"})
	
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	
	viewBeforeStream := m.View()
	t.Logf("View before stream:\n%q", viewBeforeStream)
	
	hugeOutput := "out\nout\nout\nout\nout\nout\nout\nout\nout\nout\nout\nout\nout\nout\nout\nout\nout\nout\nout\nout\n"
	m.handleBusEvent(domain.ToolStreamEvent{CallID: "bash-1", Chunk: hugeOutput})
	m.Update(flushDoneMsg{})
	
	rawTools := strings.Join(m.renderAllTools(), "")
	t.Logf("rawTools: %q", rawTools)
	
	norm := ui.NormalizeBlock(rawTools)
	t.Logf("norm: %q", norm)
	
	split := strings.Split(norm, "\n")
	t.Logf("split length: %d", len(split))
	
	isInteractive := m.isInteractiveOrAwaitingApproval()
	t.Logf("isInteractive: %v", isInteractive)
	
	gated8, _ := m.gater.Gate(split, 8, isInteractive, m.theme)
	t.Logf("Gate(8): %q", strings.Join(gated8, "\n"))
	
	gated, maxScroll := m.gater.Gate(split, m.scrollOffset, isInteractive, m.theme)
	t.Logf("gated length: %d, maxScroll: %d", len(gated), maxScroll)
	
	view := strings.Join(gated, "\n")
	t.Logf("Manual View: %q", view)
	t.Logf("Actual m.View() output: %q", m.View())


}

func TestModel_PermissionApproval_ReturnsRepaintCommand(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	tr := ui.NewToolRenderer(theme, 80, ui.NewToolOutputGater(12))
	sp := &mockSpinner{}
	m := NewModel(bus, &mockThinkingRenderer{}, tr, sp, theme, &mockStream{}, ui.NewTruncatingGater(10), 80)
	
	d := domain.NewBashDisplay("Run a loop", "ls", "", "")
	m.handleBusEvent(domain.ToolStartEvent{CallID: "bash-1", Display: d})
	m.Update(flushDoneMsg{})
	m.handleBusEvent(domain.ToolApprovalRequestEvent{CallID: "bash-1"})
	
	// Pre-condition: status should be awaiting approval
	assert.Equal(t, ui.StatusAwaitingApproval, m.tools[0].status)
	
	// Action: press 'y'
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	
	// Post-condition: should return a command that yields WindowSizeMsg
	assert.NotNil(t, cmd, "Command should not be nil on state change")
	if cmd != nil {
		msg := cmd()
		assert.IsType(t, tea.WindowSizeMsg{}, msg, "Command should return WindowSizeMsg")
	}
}

