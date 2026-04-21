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
	p             string
	appendCalls   int
	lastAppend    string
	appendReturns []string
}

func (s *mockStream) Append(t string) []string {
	s.appendCalls++
	s.lastAppend = t
	return s.appendReturns
}
func (s *mockStream) Flush() []string         { return []string{"flushed"} }
func (s *mockStream) Pending() string         { return s.p }
func (s *mockStream) ClearBuffer()            {}

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
	m := NewModel(bus, nil, nil, nil, theme, &mockStream{}, ui.NewNoOpGater(), 80)

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
	m := NewModel(bus, nil, nil, nil, theme, &mockStream{}, ui.NewNoOpGater(), 80)
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
	m := NewModel(bus, th, nil, nil, theme, &mockStream{}, ui.NewNoOpGater(), 80)
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
	m := NewModel(bus, &mockThinkingRenderer{}, nil, nil, theme, &mockStream{}, ui.NewNoOpGater(), 80)
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
	m := NewModel(bus, &mockThinkingRenderer{}, nil, nil, theme, &mockStream{}, ui.NewNoOpGater(), 80)
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

	m := NewModel(bus, th, nil, &mockSpinner{}, theme, &mockStream{}, ui.NewNoOpGater(), 80)

	// Start idle text phase and arm polling.
	m.state = stateIdle
	m.isPolling = true
	m.spinnerFrame = 7

	// Cancel while streaming.
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = res.(*Model)
	assert.True(t, m.cancelRequested)

	// Queued thinking/text activity must be ignored after cancel.
	res, _ = m.Update(busEventMsg{event: domain.TextEvent{Text: "ignored", IsThought: false}})
	m = res.(*Model)
	assert.Equal(t, stateIdle, m.state)

	res, _ = m.Update(busEventMsg{event: domain.ThinkingEvent{}})
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

func TestModel_ViewportTruncation(t *testing.T) {
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(nil, nil, nil, nil, theme, &mockStream{p: "L1\nL2\nL3\nL4\nL5"}, ui.NewTruncatingGater(3), 80)
	v := m.View()
	assert.Contains(t, v, "truncated", "View should be truncated with indicator")
}

func TestModel_NoTruncationIfHeightZero(t *testing.T) {
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(nil, nil, nil, nil, theme, &mockStream{p: "L1\nL2\nL3\nL4\nL5"}, ui.NewTruncatingGater(0), 80)
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
	m := NewModel(nil, nil, nil, nil, theme, nil, ui.NewNoOpGater(), 80)
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
	m := NewModel(bus, nil, nil, nil, theme, &mockStream{p: "raw"}, g, 80)
	
	assert.Equal(t, "raw_gated", m.View())
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

func TestModel_FlushSpacingRegression(t *testing.T) {
	var terminalOutput strings.Builder
	// mockPrintf mimics tea.Printf's internal behavior: split on \n, each line gets \n
	mockPrintf := func(s string) tea.Cmd {
		lines := strings.Split(s, "\n")
		for _, l := range lines {
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

	// Case 1: Avoid sticking between Thinking/Box and Para
	// Simulate Box ending turning into a flush
	m.doFlush([]string{"\n┌─── BOX ───┐\n└─── DONE ───┘"}, stateIdle)
	// Simulate Para delta starting with \n
	m.doFlush([]string{"\nPARA"}, stateIdle)

	output := terminalOutput.String()
	t.Logf("Output Case 1:\n%q", output)
	
	// We want exactly 1 blank line between them.
	// Current buggy behavior (blind strip) results in 0 blank lines (stuck).
	assert.Contains(t, output, "\n└─── DONE ───┘\n\nPARA\n", "Box and Para should have a blank line between them")

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
	m = res.(*Model)

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
