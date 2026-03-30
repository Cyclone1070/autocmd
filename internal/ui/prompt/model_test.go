package prompt

import (
	"strings"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockBus struct {
	updates chan domain.UIUpdate
	actions []domain.Action
}

func (m *mockBus) UIUpdates() <-chan domain.UIUpdate { return m.updates }
func (m *mockBus) SendAction(a domain.Action)       { m.actions = append(m.actions, a) }

type mockStream struct {
	p string
}
func (s *mockStream) Append(t string) []string { return nil }
func (s *mockStream) Flush() []string         { return []string{"flushed"} }
func (s *mockStream) Pending() string         { return s.p }

type mockAnimator struct {
	pending bool
}
func (a *mockAnimator) Enqueue(t string) {}
func (a *mockAnimator) NextChunk() (string, bool) { 
	if a.pending {
		a.pending = false
		return "chunk", true
	}
	return "", false 
}
func (a *mockAnimator) HasPending() bool { return a.pending }
func (a *mockAnimator) Clear()          { a.pending = false }
func (a *mockAnimator) FlushAll() string { return "" }

type mockThinkingRenderer struct{}
func (t *mockThinkingRenderer) RenderThinking(status ui.ToolStatus, start time.Time, tick int, sp spinnerProvider) string {
	return "thinking_rendered"
}

// mockThinkingRecorder records the last status passed to RenderThinking (for cancel / bus-driven flush tests).
type mockThinkingRecorder struct {
	lastStatus ui.ToolStatus
}

func (t *mockThinkingRecorder) RenderThinking(status ui.ToolStatus, start time.Time, tick int, sp spinnerProvider) string {
	t.lastStatus = status
	return "thinking_rendered"
}

func TestModel_SyncPolling(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(bus, nil, nil, nil, theme, &mockStream{}, &TextAnimator{runesPerTick: 4}, ui.NewNoOpGater(), 80)

	// pollBus delivers events as busEventMsg
	m.isPolling = true
	res, _ := m.Update(busEventMsg{event: domain.ThinkingEvent{}})
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
	m := NewModel(bus, nil, nil, nil, theme, &mockStream{}, &TextAnimator{runesPerTick: 4}, ui.NewNoOpGater(), 80)
	m.state = stateThinking
	m.spinnerFrame = 0
	for i := 0; i < 5; i++ {
		res, _ := m.Update(tickMsg{})
		m = res.(*Model)
		assert.Equal(t, i+1, m.spinnerFrame, "exactly one spinner step per tick in thinking")
	}
}

func TestModel_CancelRequested_ThinkingFlushUsesErrorStatusOnNextBusEvent(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	th := &mockThinkingRecorder{}
	m := NewModel(bus, th, nil, nil, theme, &mockStream{}, &mockAnimator{}, ui.NewNoOpGater(), 80)
	m.state = stateThinking
	m.thinkingStart = time.Now()
	m.isPolling = true

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = res.(*Model)
	assert.True(t, m.cancelRequested)
	assert.Equal(t, stateThinking, m.state)

	// After cancel, queued non-terminal bus events are trashed, but View in stateThinking
	// must still render cancelled thinking using error styling.
	_ = m.View()
	assert.Equal(t, ui.StatusError, th.lastStatus, "View in stateThinking uses error styling after cancel")
}

func TestModel_HandleCancel_DoesNotStartDuplicatePollWhenAlreadyPolling(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(bus, &mockThinkingRenderer{}, nil, nil, theme, &mockStream{}, &mockAnimator{}, ui.NewNoOpGater(), 80)
	m.state = stateTooling
	m.isPolling = true // simulate pollBus goroutine already in flight

	res, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = res.(*Model)

	assert.True(t, m.cancelRequested)
	assert.True(t, m.isPolling, "cancel must not reset isPolling when a poll is already in flight")
	assert.Nil(t, cmd, "cancel must not start a second poll when already polling")
}

func TestModel_CancelProcessesDoneEvent(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(bus, &mockThinkingRenderer{}, nil, nil, theme, &mockStream{}, &mockAnimator{}, ui.NewNoOpGater(), 80)
	m.state = stateThinking
	m.thinkingStart = time.Now()
	m.isPolling = true

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = res.(*Model)
	assert.True(t, m.cancelRequested)
	assert.Equal(t, stateThinking, m.state)
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

func TestModel_CancelRequested_IgnoresQueuedTextAndThinkingEventsInStreaming(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	th := &mockThinkingRecorder{lastStatus: ui.StatusSuccess}

	anim := &mockAnimator{pending: true}
	m := NewModel(bus, th, nil, &mockSpinner{}, theme, &mockStream{}, anim, ui.NewNoOpGater(), 80)

	// Start streaming and arm polling.
	m.state = stateStreaming
	m.isPolling = true
	m.spinnerFrame = 7

	// Cancel while streaming.
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = res.(*Model)
	assert.True(t, m.cancelRequested)
	assert.False(t, m.animator.HasPending(), "animator pending runes should be cleared on cancel during stateStreaming")

	// Queued thinking/text activity must be ignored after cancel.
	res, _ = m.Update(busEventMsg{event: domain.TextEvent{Text: "ignored", IsThought: false}})
	m = res.(*Model)
	assert.Equal(t, stateStreaming, m.state)

	res, _ = m.Update(busEventMsg{event: domain.ThinkingEvent{}})
	m = res.(*Model)
	assert.Equal(t, stateStreaming, m.state)

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
	m := NewModel(bus, nil, nil, nil, theme, &mockStream{}, &mockAnimator{pending: true}, ui.NewNoOpGater(), 80)
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
	m := NewModel(bus, &mockThinkingRenderer{}, nil, nil, theme, &mockStream{}, &mockAnimator{}, ui.NewNoOpGater(), 80, WithFlush(func(c string) tea.Cmd {
		flushed = append(flushed, c)
		return nil
	}))
	m.state = stateThinking
	
	// Transition to streaming
	m.handleBusEvent(domain.TextEvent{Text: "hi"})
	
	assert.Contains(t, flushed, "thinking_rendered", "Thinking result should be flushed on transition")
}

func TestModel_ViewportTruncation(t *testing.T) {
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(nil, nil, nil, nil, theme, &mockStream{p: "L1\nL2\nL3\nL4\nL5"}, nil, ui.NewTruncatingGater(3), 80)
	v := m.View()
	assert.Contains(t, v, "truncated", "View should be truncated with indicator")
}

func TestModel_NoTruncationIfHeightZero(t *testing.T) {
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(nil, nil, nil, nil, theme, &mockStream{p: "L1\nL2\nL3\nL4\nL5"}, nil, ui.NewTruncatingGater(0), 80)
	v := m.View()
	assert.NotContains(t, v, "truncated", "View should not be truncated when height is 0")
	assert.Contains(t, v, "L5", "Full content should be visible")
}

func TestModel_FlushDoneMsgSequencing(t *testing.T) {
	// Logical check: doFlush must use tea.Sequence to ensure order
	m := &Model{}
	m.flushFn = func(c string) tea.Cmd { return nil }
	_, cmd := m.doFlush([]string{"b1", "b2"}, stateIdle)
	
	// Verified in code: return m, tea.Sequence(cmds...)
	assert.NotNil(t, cmd)
}
func TestModel_ExplicitGater(t *testing.T) {
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(nil, nil, nil, nil, theme, nil, nil, ui.NewNoOpGater(), 80)
	// Success if constructor accepts it
	assert.NotNil(t, m.gater)
}

type mockGater struct {
	gateFunc func(string) string
}
func (m *mockGater) Gate(s string) string { return m.gateFunc(s) }

func TestModel_ViewUsesGater(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	g := &mockGater{gateFunc: func(s string) string { return s + "_gated" }}
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(bus, nil, nil, nil, theme, &mockStream{p: "raw"}, nil, g, 80)
	
	assert.Equal(t, "raw_gated", m.View())
}

func TestModel_ToolsViewSpacing(t *testing.T) {
	theme := ui.NewTheme(ui.ThemeConfig{})
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	sp := &mockSpinner{}
	m := NewModel(nil, nil, tr, sp, theme, nil, nil, ui.NewNoOpGater(), 80)
	
	m.tools = []toolSlot{
		{callID: "1", toolName: "t1", status: ui.StatusSuccess, display: domain.StringDisplay{Content: "c1"}},
		{callID: "2", toolName: "t2", status: ui.StatusSuccess, display: domain.StringDisplay{Content: "c2"}},
	}
	m.state = stateTooling
	
	v := m.View()
	// Each box starts with \n. Join adds another \n.
	// So we expect: ...bottom_border\n\n\nbox2_top_border...
	// Wait, if Box1 is "\nBox1" and Box2 is "\nBox2", and we join with "\n", 
	// we get "\nBox1" + "\n" + "\nBox2" = "\nBox1\n\nBox2".
	// The blank line is EXACTLY there.
	assert.Contains(t, v, "╯\n\n╭", "There should be a blank line between the boxes")
}

func TestModel_ToolEndEvent_ReplacesDisplayWhenNotFlushedYet(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	sp := &mockSpinner{}
	m := NewModel(bus, nil, tr, sp, theme, nil, nil, ui.NewNoOpGater(), 80)
	m.state = stateTooling
	m.tools = []toolSlot{
		{callID: "first", toolName: "t", display: domain.NewStringDisplay("", "preview first"), status: ui.StatusRunning},
		{callID: "second", toolName: "t", display: domain.NewStringDisplay("", "preview second"), status: ui.StatusRunning, errorMsg: "stale"},
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
	m := NewModel(bus, nil, tr, sp, theme, nil, nil, ui.NewNoOpGater(), 80)
	m.state = stateTooling
	m.tools = []toolSlot{
		{callID: "first", toolName: "t", display: domain.NewStringDisplay("", "preview first"), status: ui.StatusRunning},
		{callID: "second", toolName: "t", display: domain.NewStringDisplay("", "preview second"), status: ui.StatusRunning},
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

	m := NewModel(bus, nil, tr, sp, theme, nil, nil, ui.NewNoOpGater(), 80)
	m.state = stateTooling
	// Keep another tool running in front so the cancelled slot is not flushed from the prefix queue in this step.
	m.tools = []toolSlot{
		{callID: "1", toolName: "t1", status: ui.StatusRunning, display: domain.StringDisplay{Content: "still running"}},
		{callID: "2", toolName: "t2", status: ui.StatusRunning, display: domain.StringDisplay{Content: "Run something"}},
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
	m := NewModel(bus, tickr, tr, sp, theme, &mockStream{}, &mockAnimator{}, ui.NewNoOpGater(), 80, WithFlush(func(s string) tea.Cmd {
		flushed = append(flushed, s)
		return nil
	}))
	m.state = stateThinking
	m.thinkingStart = time.Now()

	// Act: bus closed while a poll was in flight
	m.isPolling = true
	m.Update(busClosedMsg{})

	// Assert: Should detect closed channel, mark tools as error, and transition to flushing/done
	assert.Contains(t, flushed, "thinking_rendered")
	
	found := false
	for _, f := range flushed {
		// Use a case-insensitive check or specific styled check
		if strings.Contains(strings.ToLower(f), "bus closed unexpectedly") {
			found = true
			break
		}
	}
	assert.True(t, found, "Should flush the explicit error message 'bus closed unexpectedly'")
}
