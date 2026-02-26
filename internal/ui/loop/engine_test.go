package loop

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
)

type engineMockRenderer struct {
	renderFunc func(markdown string) (string, error)
}

func (m *engineMockRenderer) Render(markdown string) (string, error) {
	if m.renderFunc != nil {
		return m.renderFunc(markdown)
	}
	return markdown, nil
}

// executeCmd recursively unwraps batches and executes each command.
// It uses a small timeout for each command to avoid hanging on waitForEvent.
func executeCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}

	done := make(chan tea.Msg, 1)
	go func() {
		defer func() { _ = recover() }()
		done <- cmd()
	}()

	var msgs []tea.Msg
	select {
	case msg := <-done:
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, bcmd := range batch {
				msgs = append(msgs, executeCmd(bcmd)...)
			}
		} else if fmt.Sprintf("%T", msg) == "tea.sequenceMsg" {
			v := reflect.ValueOf(msg)
			for i := 0; i < v.Len(); i++ {
				cmd := v.Index(i).Interface().(tea.Cmd)
				msgs = append(msgs, executeCmd(cmd)...)
			}
		} else {
			msgs = append(msgs, msg)
		}
	case <-time.After(50 * time.Millisecond):
		// Skip blocking commands
	}
	return msgs
}

// outputTracker captures history and side effects for assertions.
type outputTracker struct {
	history []string
	signals []string
}

func (o *outputTracker) capture(cmd tea.Cmd) {
	msgs := executeCmd(cmd)
	for _, msg := range msgs {
		o.handleMsg(msg)
	}
}

func (o *outputTracker) handleMsg(msg tea.Msg) {
	if msg == nil {
		return
	}
	switch m := msg.(type) {
	case tea.BatchMsg:
		for _, c := range m {
			o.capture(c)
		}
	case eventMsg, streamTickMsg, spinner.TickMsg:
		// Don't track loop-driving messages
	default:
		// Capture everything else as a side-effect
		s := fmt.Sprintf("%v", m)
		o.history = append(o.history, s)
	}
}

// NewTestModel creates a model with a mock renderer for literal string matching.
func NewTestModel(events chan domain.Event) *Model {
	cfg := config.DefaultConfig().UI
	cfg.ChatWindowWidth = 80
	m := NewModel(events, cfg, WithFlush(func(content string) tea.Cmd {
		if currentTracker != nil {
			currentTracker.signals = append(currentTracker.signals, content)
		}
		// We still execute the actual printf so history capture works too
		return tea.Printf("%s", content)
	}))
	m.stream.renderer = &engineMockRenderer{}
	return m
}

// msgQueue is a simple queue for processing messages sequentially in tests.
type msgQueue struct {
	m    *Model
	msgs []tea.Msg
}

func (q *msgQueue) push(cmds ...tea.Cmd) {
	for _, cmd := range cmds {
		q.msgs = append(q.msgs, executeCmd(cmd)...)
	}
}

func (q *msgQueue) step() bool {
	if len(q.msgs) == 0 {
		return false
	}
	msg := q.msgs[0]
	q.msgs = q.msgs[1:]

	if currentTracker != nil {
		currentTracker.handleMsg(msg)
	}

	tm, cmd := q.m.Update(msg)
	q.m = tm.(*Model)
	if cmd != nil {
		// Only recurse for non-spinner ticks to avoid infinite loops
		if _, ok := msg.(spinner.TickMsg); !ok {
			q.push(cmd)
		}
	}
	return true
}

func (q *msgQueue) drain() {
	for q.step() {
	}
}

var currentTracker *outputTracker

func TestModel_Update_SmoothStreaming(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewModel(events, config.DefaultConfig().UI)
	renderer := &engineMockRenderer{}
	m.stream = NewStream(renderer)

	// Send a TextEvent with multiple chunks worth of characters
	longText := "12345678" // 8 chars = 2 chunks
	msg := eventMsg{event: domain.TextEvent{Text: longText}}

	// 1. Send text event
	tm, cmd := m.Update(msg)
	m = tm.(*Model)
	assert.Len(t, m.queue, 1)
	assert.Equal(t, "5678", m.queue[0].(domain.TextEvent).Text)
	assert.NotNil(t, cmd)

	// Extract tick from batch
	msgs := executeCmd(cmd)
	tickMsg := extractTickFromMsgs(msgs)
	assert.NotNil(t, tickMsg)

	// 2. Process Tick 1 -> should process remaining "5678" and leave queue empty
	tm, cmd = m.Update(tickMsg)
	m = tm.(*Model)
	assert.Empty(t, m.queue)
	assert.Equal(t, "12345678", m.stream.buffer)

	// Since remainder was "", no more ticks should be scheduled (but waitForEvent is added)
	msgs2 := executeCmd(cmd)
	for _, bmsg := range msgs2 {
		_, ok := bmsg.(streamTickMsg)
		assert.False(t, ok, "Should not have streamTickMsg in final batch")
	}
}

func extractTickFromMsgs(msgs []tea.Msg) tea.Msg {
	for _, msg := range msgs {
		if _, ok := msg.(streamTickMsg); ok {
			return msg
		}
	}
	return nil
}

func TestModel_Update_TextEvent(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewModel(events, config.DefaultConfig().UI)
	renderer := &engineMockRenderer{}
	m.stream = NewStream(renderer)

	msg := eventMsg{event: domain.TextEvent{Text: "Hello\n\nWorld"}}
	_, cmd := m.Update(msg)

	assert.NotNil(t, cmd)
	// cmd should be a batch {streamTick} or {streamTick, printf, ...}
	// waitForEvent (eventMsg) is NOT present because isStreaming is true.
	msgs := executeCmd(cmd)
	foundTick := false
	foundWait := false
	for _, bmsg := range msgs {
		switch bmsg.(type) {
		case streamTickMsg:
			foundTick = true
		case eventMsg:
			foundWait = true
		}
	}
	assert.True(t, foundTick, "Should contain streamTickMsg")
	assert.False(t, foundWait, "Should NOT contain waitForEvent (eventMsg) - it is suppressed by isWaiting")
}

func TestModel_View_Truncation(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewModel(events, config.DefaultConfig().UI)
	m.height = 5

	longText := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10"
	renderer := &engineMockRenderer{}
	m.stream = NewStream(renderer)
	_, _ = m.stream.Append(longText)

	view := m.View()

	lines := 0
	for _, char := range view {
		if char == '\n' {
			lines++
		}
	}
	assert.LessOrEqual(t, lines+1, m.height)
	assert.Contains(t, view, "\n  ▲ [7 lines truncated]")
}

func TestModel_Update_ThinkingEvent(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewModel(events, config.DefaultConfig().UI)

	// 1. Send ThinkingEvent
	tm, cmd := m.Update(eventMsg{event: domain.ThinkingEvent{}})
	m = tm.(*Model)
	assert.True(t, m.isThinking)
	assert.NotZero(t, m.thinkStart)
	assert.NotNil(t, cmd)

	events <- domain.DoneEvent{}
	mMsg := cmd()
	batch, ok := mMsg.(tea.BatchMsg)
	assert.True(t, ok, "Should return a batch")

	foundSpinner := false
	foundWait := false
	for _, bcmd := range batch {
		if bcmd == nil {
			continue
		}
		bmsg := bcmd()
		// spinner.Tick returns spinner.TickMsg
		if _, ok := bmsg.(spinner.TickMsg); ok {
			foundSpinner = true
		} else if _, ok := bmsg.(eventMsg); ok {
			foundWait = true
		}
	}
	assert.True(t, foundSpinner, "Should contain spinner.Tick")
	assert.True(t, foundWait, "Should contain waitForEvent (eventMsg)")

	// 2. Send TextEvent to finish thinking
	msg := eventMsg{event: domain.TextEvent{Text: "Thinking done"}}
	tm, cmd = m.Update(msg)
	m = tm.(*Model)

	assert.False(t, m.isThinking)
	assert.NotEmpty(t, m.thinkStart)
	assert.NotNil(t, cmd)

	// Concrete assertion on Printf content
	select {
	case events <- domain.DoneEvent{}:
	default:
	}
	msgs := executeCmd(cmd)
	foundPrintf := false
	for _, bmsg := range msgs {
		s := fmt.Sprintf("%v", bmsg)
		if strings.Contains(s, "Thought for") {
			foundPrintf = true
			break
		}
	}
	assert.True(t, foundPrintf, "Should have found Printf in batch")
}

func TestModel_Update_EventLoopContinuity(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewModel(events, config.DefaultConfig().UI)

	// 1. ThinkingEvent must batch waitForEvent
	_, cmd := m.Update(eventMsg{event: domain.ThinkingEvent{}})
	assert.NotNil(t, cmd)

	events <- domain.DoneEvent{}
	mMsg := cmd()

	batch, ok := mMsg.(tea.BatchMsg)
	assert.True(t, ok, "ThinkingEvent should return a Batch to keep polling")

	foundWait := false
	for _, bcmd := range batch {
		if bcmd == nil {
			continue
		}
		bmsg := bcmd()
		if _, ok := bmsg.(eventMsg); ok {
			foundWait = true
			break
		}
	}
	assert.True(t, foundWait, "Should contain waitForEvent (eventMsg)")

	// 2. TextEvent (start of streaming) must NOT batch waitForEvent (it waits for streamDone)
	m = NewModel(events, config.DefaultConfig().UI)
	_, cmd = m.Update(eventMsg{event: domain.TextEvent{Text: "Hello"}})
	assert.NotNil(t, cmd)

	msgs := executeCmd(cmd)
	foundWait = false
	for _, bmsg := range msgs {
		if _, ok := bmsg.(eventMsg); ok {
			foundWait = true
			break
		}
	}
	assert.False(t, foundWait, "TextEvent should NOT return a Batch to keep polling (blocking leapfrog)")
}

func TestModel_Update_KeyMsg_Quit_InstantWipe(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewModel(events, config.DefaultConfig().UI)

	// 1. Fill model with busy state
	m.isThinking = true
	m.queue = []domain.Event{domain.TextEvent{Text: "pending text"}}
	m.isStreaming = true
	m.activeTools["busy-tool"] = &toolState{
		id:      "busy-tool",
		status:  ui.StatusRunning,
		display: domain.StringDisplay("Running busy-tool..."),
	}
	m.toolOrder = []string{"busy-tool"}

	// 2. Trigger Ctrl+C
	tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = tm.(*Model)

	// 3. ASSERT: Instant reset
	assert.False(t, m.isThinking, "Thinking state should be wiped instantly on Ctrl+C")
	assert.Empty(t, m.queue, "Queue should be wiped instantly on Ctrl+C")
	assert.False(t, m.isStreaming, "Streaming state should be wiped instantly on Ctrl+C")
	assert.Empty(t, m.activeTools, "Active tools should be cleared after flushing")

	assert.Empty(t, m.printQueue, "printQueue should be empty because it was flushed to tea.Cmd")

	// 5. ASSERT: Quit signal present and cancellation rendered
	tracker := &outputTracker{}
	tracker.capture(cmd)

	foundCancelledTool := false
	foundQuit := false
	for _, h := range tracker.history {
		if strings.Contains(h, "quitMsg") || strings.Contains(h, "{}") {
			foundQuit = true
		}
		if strings.Contains(h, "Cancelled") {
			foundCancelledTool = true
		}
	}
	assert.True(t, foundCancelledTool, "Should find 'Cancelled' tool output in historical tracker.history")
	assert.True(t, foundQuit, "Should find Quit message in history/commands")
}

func TestModel_Update_ToolLifecycle(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewModel(events, config.DefaultConfig().UI)

	// 1. Tool Start
	startEv := domain.ToolStartEvent{
		CallID: "123",
		Display: domain.ShellDisplay{
			Header:  "Run tests",
			Command: "go test ./...",
		},
	}
	m2, cmd := m.Update(eventMsg{event: startEv})
	m = m2.(*Model)

	assert.NotNil(t, m.activeTools["123"])
	assert.Equal(t, ui.StatusRunning, m.activeTools["123"].status)

	events <- domain.DoneEvent{}
	msgs := executeCmd(cmd)
	foundSpinner := false
	foundWait := false
	for _, msg := range msgs {
		if _, ok := msg.(spinner.TickMsg); ok {
			foundSpinner = true
		} else if _, ok := msg.(eventMsg); ok {
			foundWait = true
		}
	}
	assert.True(t, foundSpinner, "Should contain spinner.Tick")
	assert.True(t, foundWait, "Should contain waitForEvent (eventMsg)")

	// 2. Tool Stream
	streamEv := domain.ToolStreamEvent{
		CallID: "123",
		Chunk:  "PASS\n",
	}
	m2, cmd = m.Update(eventMsg{event: streamEv})
	m = m2.(*Model)
	assert.Contains(t, m.activeTools["123"].output, "PASS")

	events <- domain.DoneEvent{}
	msgs = executeCmd(cmd)
	foundWait = false
	for _, bmsg := range msgs {
		if _, ok := bmsg.(eventMsg); ok {
			foundWait = true
			break
		}
	}
	assert.True(t, foundWait, "Should contain waitForEvent (eventMsg)")

	// 3. Tool End
	endEv := domain.ToolEndEvent{
		CallID: "123",
		Error:  "",
	}
	m2, cmd = m.Update(eventMsg{event: endEv})
	m = m2.(*Model)

	assert.Empty(t, m.activeTools)
	assert.NotNil(t, cmd)

	events <- domain.DoneEvent{}
	msgs = executeCmd(cmd)
	foundPrintf := false
	foundWait = false
	for _, bmsg := range msgs {
		s := fmt.Sprintf("%v", bmsg)
		if strings.Contains(s, "Run tests") { // Header is in ShellDisplay
			foundPrintf = true
		} else if _, ok := bmsg.(eventMsg); ok {
			foundWait = true
		}
	}
	assert.True(t, foundPrintf, "Should contain Printf (tool output)")
	assert.True(t, foundWait, "Should contain waitForEvent (eventMsg)")
}

func TestModel_Update_EventOrdering_SequentialHistory(t *testing.T) {
	events := make(chan domain.Event, 20)
	m := NewModel(events, config.DefaultConfig().UI)
	renderer := &engineMockRenderer{}
	m.stream = NewStream(renderer)
	tracker := &outputTracker{}

	currentTracker = tracker
	defer func() { currentTracker = nil }()

	q := &msgQueue{m: m}
	q.push(func() tea.Msg { return eventMsg{event: domain.ThinkingEvent{}} })
	q.drain()
	_ = q.m

	text := "Block 1\n\nBlock 2\n\n"
	q.push(func() tea.Msg { return eventMsg{event: domain.TextEvent{Text: text}} })
	q.drain()
	_ = q.m

	// 2. Send ToolStartEvent immediately
	toolEv := domain.ToolStartEvent{
		CallID:  "tool-seq",
		Display: domain.StringDisplay("SEQUENTIAL TOOL"),
	}
	q.push(func() tea.Msg { return eventMsg{event: toolEv} })
	q.drain()
	_ = q.m

	// 3. Send ToolEndEvent immediately
	endEv := domain.ToolEndEvent{CallID: "tool-seq"}
	q.push(func() tea.Msg { return eventMsg{event: endEv} })
	q.drain()
	_ = q.m

	// 4. Drain the stream
	for range 30 {
		q.push(func() tea.Msg { return streamTickMsg{} })
		q.drain()
	}
	_ = q.m

	// 6. ASSERT FINAL HISTORY ORDER
	// Expected Order:
	// 1. "Thought for" (since thinking was before text)
	// 2. "Block 1"
	// 3. "Block 2"
	// 4. "SEQUENTIAL TOOL"
	var fullHistory string
	for _, h := range tracker.history {
		fullHistory += h + "|"
	}

	idxThink := strings.Index(fullHistory, "Thought for")
	idx1 := strings.Index(fullHistory, "Block 1")
	idx2 := strings.Index(fullHistory, "Block 2")
	idxT := strings.Index(fullHistory, "SEQUENTIAL TOOL")

	assert.NotEqual(t, -1, idxThink, "Thinking completion should be in history. Full: %s", fullHistory)
	assert.NotEqual(t, -1, idx1, "Block 1 should be in history. Full: %s", fullHistory)
	assert.NotEqual(t, -1, idx2, "Block 2 should be in history. Full: %s", fullHistory)
	assert.NotEqual(t, -1, idxT, "Tool should be in history. Full: %s", fullHistory)

	// Order validation
	assert.True(t, idxThink < idx1, "Thinking must finish before Block 1")
	assert.True(t, idx1 < idx2, "Block 1 must come before Block 2")
	assert.True(t, idx2 < idxT, "Block 2 must come before Tool output")
}

func TestModel_Update_DoneEvent_Flush(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewModel(events, config.DefaultConfig().UI)
	renderer := &engineMockRenderer{}
	m.stream = NewStream(renderer)
	tracker := &outputTracker{}

	currentTracker = tracker
	defer func() { currentTracker = nil }()

	// 1. Send text but don't tick it yet
	text := "Final unflushed text"
	q := &msgQueue{m: m}
	q.push(func() tea.Msg { return eventMsg{event: domain.TextEvent{Text: text}} })
	q.drain()

	// 2. Send DoneEvent
	q.push(func() tea.Msg { return eventMsg{event: domain.DoneEvent{}} })
	q.drain()
	_ = q.m

	// 3. ASSERT: History contains the flushed text
	found := false
	for _, h := range tracker.history {
		if strings.Contains(h, "Final unflushed text") {
			found = true
			break
		}
	}
	var full string
	for _, h := range tracker.history {
		full += h + "|"
	}
	assert.True(t, found, "DoneEvent should flush remaining text to history before quitting. Full: %s", full)
}

func TestModel_Update_Interleaved_ThinkingLeapfrog_Done(t *testing.T) {
	events := make(chan domain.Event, 20)
	m := NewModel(events, config.DefaultConfig().UI)
	renderer := &engineMockRenderer{}
	m.stream = NewStream(renderer)
	tracker := &outputTracker{}
	currentTracker = tracker
	defer func() { currentTracker = nil }()

	q := &msgQueue{m: m}

	// 1. Start Text 1 (Turn 1)
	q.push(func() tea.Msg { return eventMsg{event: domain.TextEvent{Text: "Text 1"}} })

	// 2. Step once to start ticking. "Text" is taken, " 1" remains.
	q.step()
	assert.NotEmpty(t, q.m.queue, "Text should still be in queue")

	// 3. Send Thinking (Turn 2) - should be buffered
	q.push(func() tea.Msg { return eventMsg{event: domain.ThinkingEvent{}} })

	// 4. Send Text 2 (Turn 2) - currently leapfrogs
	q.push(func() tea.Msg { return eventMsg{event: domain.TextEvent{Text: "Text 2"}} })

	// 5. Send Done
	q.push(func() tea.Msg { return eventMsg{event: domain.DoneEvent{}} })

	// 6. Drain everything
	q.drain()

	fullHistory := strings.Join(tracker.history, "|")

	// Correct Order: Text 1 | Thought for | Text 2
	// Buggy Order:   Text 1Text 2 | Thought for | Done flushing
	idx1 := strings.Index(fullHistory, "Text 1")
	idxThink := strings.Index(fullHistory, "Thought for")
	idx2 := strings.Index(fullHistory, "Text 2")

	assert.NotEqual(t, -1, idx1, "Text 1 missing")
	assert.NotEqual(t, -1, idxThink, "Thinking duration missing")
	assert.NotEqual(t, -1, idx2, "Text 2 missing")

	assert.True(t, idx1 < idxThink, "Text 1 must come before Thinking. Full: %s", fullHistory)
	assert.True(t, idxThink < idx2, "Thinking must come before Text 2 (No leapfrogging!). Full: %s", fullHistory)
}

func TestModel_Update_Interleaved_ToolLeapfrog_Done(t *testing.T) {
	events := make(chan domain.Event, 20)
	m := NewModel(events, config.DefaultConfig().UI)
	renderer := &engineMockRenderer{}
	m.stream = NewStream(renderer)
	tracker := &outputTracker{}
	currentTracker = tracker
	defer func() { currentTracker = nil }()

	q := &msgQueue{m: m}

	// 1. Text 1 (Turn 1)
	q.push(func() tea.Msg { return eventMsg{event: domain.TextEvent{Text: "Text 1"}} })
	q.step() // Take "Text", " 1" remains in queue

	// 2. Tool (Turn 1)
	q.push(func() tea.Msg {
		return eventMsg{event: domain.ToolStartEvent{CallID: "TC1", Display: domain.StringDisplay("TOOL 1")}}
	})
	q.push(func() tea.Msg { return eventMsg{event: domain.ToolEndEvent{CallID: "TC1"}} })

	// 3. Text 2 (Turn 2) - currently leapfrogs
	q.push(func() tea.Msg { return eventMsg{event: domain.TextEvent{Text: "Text 2"}} })

	// 4. Done
	q.push(func() tea.Msg { return eventMsg{event: domain.DoneEvent{}} })

	q.drain()

	fullHistory := strings.Join(tracker.history, "|")

	idx1 := strings.Index(fullHistory, "Text 1")
	idxTool := strings.Index(fullHistory, "TOOL 1")
	idx2 := strings.Index(fullHistory, "Text 2")

	assert.NotEqual(t, -1, idx1, "Text 1 missing")
	assert.NotEqual(t, -1, idxTool, "Tool 1 missing")
	assert.NotEqual(t, -1, idx2, "Text 2 missing")

	assert.True(t, idx1 < idxTool, "Text 1 must come before Tool. Full: %s", fullHistory)
	assert.True(t, idxTool < idx2, "Tool must come before Text 2 (No leapfrogging!). Full: %s", fullHistory)
}

func TestModel_Update_Interleaved_GracefulExit_Done(t *testing.T) {
	events := make(chan domain.Event, 20)
	m := NewModel(events, config.DefaultConfig().UI)
	renderer := &engineMockRenderer{}
	m.stream = NewStream(renderer)
	tracker := &outputTracker{}
	currentTracker = tracker
	defer func() { currentTracker = nil }()

	q := &msgQueue{m: m}

	// 1. Send text that will take multiple ticks
	q.push(func() tea.Msg { return eventMsg{event: domain.TextEvent{Text: "Slowly being printed text"}} })

	// 2. Send Done immediately
	q.push(func() tea.Msg { return eventMsg{event: domain.DoneEvent{}} })

	// 3. Drain and ensure the app didn't quit until all ticks are done
	q.drain()

	fullHistory := strings.Join(tracker.history, "|")
	assert.Contains(t, fullHistory, "Slowly being printed text", "Application should not exit until busy text is fully printed")
}

func TestModel_Update_Interleaved_ToolSequentiality_Done(t *testing.T) {
	events := make(chan domain.Event, 20)
	m := NewModel(events, config.DefaultConfig().UI)
	renderer := &engineMockRenderer{}
	m.stream = NewStream(renderer)
	tracker := &outputTracker{}
	currentTracker = tracker
	defer func() { currentTracker = nil }()

	q := &msgQueue{m: m}

	// 1. Text (Turn 1)
	q.push(func() tea.Msg { return eventMsg{event: domain.TextEvent{Text: "Busy text"}} })
	q.step()

	// 2. Tool 1
	q.push(func() tea.Msg {
		return eventMsg{event: domain.ToolStartEvent{CallID: "T1", Display: domain.StringDisplay("TOOL 1")}}
	})
	q.push(func() tea.Msg { return eventMsg{event: domain.ToolEndEvent{CallID: "T1"}} })

	// 3. Tool 2
	q.push(func() tea.Msg {
		return eventMsg{event: domain.ToolStartEvent{CallID: "T2", Display: domain.StringDisplay("TOOL 2")}}
	})
	q.push(func() tea.Msg { return eventMsg{event: domain.ToolEndEvent{CallID: "T2"}} })

	// 4. Done
	q.push(func() tea.Msg { return eventMsg{event: domain.DoneEvent{}} })

	q.drain()

	fullHistory := strings.Join(tracker.history, "|")

	idx1 := strings.Index(fullHistory, "TOOL 1")
	idx2 := strings.Index(fullHistory, "TOOL 2")

	assert.NotEqual(t, -1, idx1, "TOOL 1 missing")
	assert.NotEqual(t, -1, idx2, "TOOL 2 missing")
	assert.True(t, idx1 < idx2, "TOOL 1 must precede TOOL 2 in history. Full: %s", fullHistory)
}

func TestFlush_Natural(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewTestModel(events)
	tracker := &outputTracker{}
	currentTracker = tracker
	defer func() { currentTracker = nil }()

	// 1. Send text that contains a finished block (H1) and an unfinished tail (Block 2).
	tm, cmd := m.Update(eventMsg{event: domain.TextEvent{Text: "## H1\n\nBETA"}})
	m = tm.(*Model)
	tracker.capture(cmd)

	// 2. Pulse enough ticks to process "## H1\n\n".
	for range 2 {
		tm, cmd = m.Update(streamTickMsg{})
		m = tm.(*Model)
		tracker.capture(cmd)
	}

	// Assertion: "H1" signal should be emitted (stripped of margin by Stream).
	allSignals := strings.Join(tracker.signals, "")
	assert.Contains(t, allSignals, "## H1", "Safe block (H1) should emit a flush signal")

	// Assertion: "BETA" should NOT be in signals yet.
	assert.NotContains(t, allSignals, "BETA", "Unsafe tail (BETA) should not emit a flush signal yet")
	// Note: It might be in textQueue or stream.buffer depending on exact rune counts
}

func TestFlush_OnThinkingStart(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewTestModel(events)
	tracker := &outputTracker{}
	currentTracker = tracker
	defer func() { currentTracker = nil }()

	// 1. Send text and pulse it into the buffer (as unsafe tail)
	tm, cmd := m.Update(eventMsg{event: domain.TextEvent{Text: "ALPHA"}})
	m = tm.(*Model)
	tracker.capture(cmd)
	for range 5 {
		tm, cmd = m.Update(streamTickMsg{})
		m = tm.(*Model)
		tracker.capture(cmd)
	}
	assert.Contains(t, m.stream.buffer, "ALPHA")
	assert.Empty(t, tracker.signals)

	// 2. Send Thinking Event
	tm, cmd = m.Update(eventMsg{event: domain.ThinkingEvent{}})
	_ = tm.(*Model)

	tracker.capture(cmd)

	// Assertion: "ALPHA" signal should be emitted
	allSignals := strings.Join(tracker.signals, "")
	assert.Contains(t, allSignals, "ALPHA", "ThinkingStart should emit flush signal for pending buffer")
}

func TestFlush_OnThinkingEnd(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewTestModel(events)
	tracker := &outputTracker{}
	currentTracker = tracker
	defer func() { currentTracker = nil }()

	m.isThinking = true
	tm, cmd := m.Update(eventMsg{event: domain.TextEvent{Text: "BETA"}})
	m = tm.(*Model)
	tracker.capture(cmd)
	for range 5 {
		tm, cmd = m.Update(streamTickMsg{})
		m = tm.(*Model)
		tracker.capture(cmd)
	}

	// 2. Transition out of thinking
	// We'll simulate a substantive event that ends thinking
	tm, cmd = m.Update(eventMsg{event: domain.ToolStartEvent{CallID: "T1"}})
	_ = tm.(*Model)
	tracker.capture(cmd)

	// Assertion: "BETA" signal should be emitted
	allSignals := strings.Join(tracker.signals, "")
	assert.Contains(t, allSignals, "BETA", "ThinkingEnd should emit flush signal for pending buffer")
}

func TestFlush_OnToolStart(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewTestModel(events)
	tracker := &outputTracker{}
	currentTracker = tracker
	defer func() { currentTracker = nil }()

	tm, cmd := m.Update(eventMsg{event: domain.TextEvent{Text: "GAMMA"}})
	m = tm.(*Model)
	tracker.capture(cmd)
	for range 5 {
		tm, cmd = m.Update(streamTickMsg{})
		m = tm.(*Model)
		tracker.capture(cmd)
	}
	assert.Empty(t, tracker.signals)

	// 2. Start Tool
	tm, cmd = m.Update(eventMsg{event: domain.ToolStartEvent{CallID: "T1"}})
	_ = tm.(*Model)
	tracker.capture(cmd)

	allSignals := strings.Join(tracker.signals, "")
	assert.Contains(t, allSignals, "GAMMA", "ToolStart should emit flush signal for pending buffer")
}

func TestFlush_OnToolEnd(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewTestModel(events)
	tracker := &outputTracker{}
	currentTracker = tracker
	defer func() { currentTracker = nil }()

	m.activeTools["T1"] = &toolState{id: "T1", status: ui.StatusRunning}
	m.toolOrder = []string{"T1"}
	tm, cmd := m.Update(eventMsg{event: domain.TextEvent{Text: "DELTA"}})
	m = tm.(*Model)
	tracker.capture(cmd)
	for range 5 {
		tm, cmd = m.Update(streamTickMsg{})
		m = tm.(*Model)
		tracker.capture(cmd)
	}
	assert.Empty(t, tracker.signals)

	// 2. End Tool
	tm, cmd = m.Update(eventMsg{event: domain.ToolEndEvent{CallID: "T1"}})
	_ = tm.(*Model)
	tracker.capture(cmd)

	allSignals := strings.Join(tracker.signals, "")
	assert.Contains(t, allSignals, "DELTA", "ToolEnd should emit flush signal for pending buffer")
}

func TestFlush_OnDone(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewTestModel(events)
	tracker := &outputTracker{}
	currentTracker = tracker
	defer func() { currentTracker = nil }()

	tm, cmd := m.Update(eventMsg{event: domain.TextEvent{Text: "EPSILON"}})
	m = tm.(*Model)
	tracker.capture(cmd)
	for range 5 {
		tm, cmd = m.Update(streamTickMsg{})
		m = tm.(*Model)
		tracker.capture(cmd)
	}
	assert.Empty(t, tracker.signals)

	// 2. Done
	tm, cmd = m.Update(eventMsg{event: domain.DoneEvent{}})
	_ = tm.(*Model)
	tracker.capture(cmd)

	allSignals := strings.Join(tracker.signals, "")
	assert.Contains(t, allSignals, "EPSILON", "DoneEvent should emit flush signal for remaining buffer")
}

func TestFlush_OnInterrupt(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewTestModel(events)
	tracker := &outputTracker{}
	currentTracker = tracker
	defer func() { currentTracker = nil }()

	// 1. Send text and pulse once to split between buffer and queue
	tm, _ := m.Update(eventMsg{event: domain.TextEvent{Text: "KEEPDISCARD"}})
	m = tm.(*Model)
	// Pulse 4 runes: "KEEP" -> buffer, "DISC", "ARD" -> queue
	tm, _ = m.Update(streamTickMsg{})
	m = tm.(*Model)

	assert.Equal(t, "KEEPDISC", m.stream.buffer)
	assert.Len(t, m.queue, 1)
	assert.Equal(t, "ARD", m.queue[0].(domain.TextEvent).Text)
	assert.Empty(t, tracker.signals)

	// 2. Ctrl+C (Emergency Exit)
	tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = tm.(*Model)
	tracker.capture(cmd)

	// Assertion: Buffer ("KEEP") should be signaled. Queue ("DISCARD") should NOT.
	allSignals := strings.Join(tracker.signals, "")
	assert.Contains(t, allSignals, "KEEP", "Buffer tail should be signaled on interrupt")
	assert.NotContains(t, allSignals, "DISCARD", "queue should be discarded on interrupt")
	assert.Empty(t, m.queue, "Interrupt should leave queue empty")
}

func TestIssue_FinalizerBypass_SpinnerTick(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewTestModel(events)
	m.isThinking = true // Ensure spinner.Update is called
	tracker := &outputTracker{}
	currentTracker = tracker
	defer func() { currentTracker = nil }()

	// 1. Manually seed printQueue as if a handler forgot to flush
	// (or as if we want to ensure any branch flushes)
	m.printQueue = []string{"TRAPPED"}

	// 2. Send spinner.TickMsg
	tm, cmd := m.Update(spinner.TickMsg{})
	m = tm.(*Model)
	tracker.capture(cmd)

	// Assertion: In the current bugged state, tracker.signals will be empty
	// because return m, cmd bypasses continueEventLoop().
	assert.Contains(t, strings.Join(tracker.signals, ""), "TRAPPED", "Spinner tick should still trigger finalization if output is pending")
	assert.Empty(t, m.printQueue, "printQueue should be cleared even on spinner tick")
}

func TestIssue_RenderingFallback(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewTestModel(events)

	// 1. Setup a renderer that fails
	m.stream.renderer = &engineMockRenderer{
		renderFunc: func(markdown string) (string, error) {
			return "", fmt.Errorf("glamour failure")
		},
	}
	tracker := &outputTracker{}
	currentTracker = tracker
	defer func() { currentTracker = nil }()

	// 2. Put something in the buffer
	m.stream.buffer = "RAW_MARKDOWN"

	// 3. Trigger a flush (e.g. via ThinkingEvent)
	tm, cmd := m.Update(eventMsg{event: domain.ThinkingEvent{}})
	_ = tm.(*Model)
	tracker.capture(cmd)

	// Assertion: Current implementation ignores error and returns empty safe slice.
	// We want it to contain the raw markdown.
	allSignals := strings.Join(tracker.signals, "")
	assert.Contains(t, allSignals, "RAW_MARKDOWN", "Should fallback to raw markdown on rendering error")
}

func TestIssue_PureView_DeterministicDuration(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewModel(events, config.DefaultConfig().UI)

	m.isThinking = true
	m.thinkStart = time.Now().Add(-5 * time.Second)

	// Set the cached duration manually
	m.thinkingDuration = 5 * time.Second

	v1 := m.View()
	assert.Contains(t, v1, "5s", "View should reflect cached duration")

	// Even if time passes, View should NOT change if no Update occurred
	// (Simulate time pass by manually shifting thinkStart further back)
	m.thinkStart = m.thinkStart.Add(-10 * time.Second)
	v2 := m.View()
	assert.Equal(t, v1, v2, "View must be pure and not change without an Update")
}

func TestModel_Spinner_Clockwise(t *testing.T) {
	events := make(chan domain.Event)
	m := NewModel(events, config.DefaultConfig().UI)

	expectedFrames := []string{"⣾ ", "⣷ ", "⣯ ", "⣟ ", "⡿ ", "⢿ ", "⣻ ", "⣽ "}
	assert.Equal(t, expectedFrames, m.spinner.Spinner.Frames)
}

func TestModel_ParallelTools(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewTestModel(events)

	// 1. Start Tool A
	tm, _ := m.Update(eventMsg{event: domain.ToolStartEvent{
		CallID:  "A",
		Display: domain.StringDisplay("Tool A"),
	}})
	m = tm.(*Model)

	// 2. Start Tool B
	tm, _ = m.Update(eventMsg{event: domain.ToolStartEvent{
		CallID:  "B",
		Display: domain.StringDisplay("Tool B"),
	}})
	m = tm.(*Model)

	// ASSERT: Both should be in View
	view := m.View()
	assert.Contains(t, view, "Tool A")
	assert.Contains(t, view, "Tool B")

	// 3. Stream to Tool A
	tm, _ = m.Update(eventMsg{event: domain.ToolStreamEvent{
		CallID: "A",
		Chunk:  " output A",
	}})
	m = tm.(*Model)

	// 4. End Tool B (Parallel finishing)
	tracker := &outputTracker{}
	currentTracker = tracker
	defer func() { currentTracker = nil }()

	tm, cmd := m.Update(eventMsg{event: domain.ToolEndEvent{
		CallID: "B",
	}})
	m = tm.(*Model)
	tracker.capture(cmd)

	// ASSERT: Tool B finished but pinned to status because blocked by A
	view = m.View()
	assert.Contains(t, view, "Tool A")
	assert.Contains(t, view, "Tool B")

	// ASSERT: Tool B removed from active? No, it should still be in activeTools
	// until flushed, but status is now Success.
	assert.Equal(t, ui.StatusSuccess, m.activeTools["B"].status)
	assert.Contains(t, m.toolOrder, "B")

	// ASSERT: Tool B should NOT be in pending/printf output yet (blocked by A)
	foundB := false
	for _, h := range tracker.history {
		if strings.Contains(h, "Tool B") {
			foundB = true
			break
		}
	}
	assert.False(t, foundB, "Tool B should NOT be in printed history yet")
}

func TestModel_OrderedFlushing(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewTestModel(events)

	// 1. Start 3 tools: A, B, C
	for _, id := range []string{"A", "B", "C"} {
		tm, _ := m.Update(eventMsg{event: domain.ToolStartEvent{
			CallID:  id,
			Display: domain.StringDisplay("Tool " + id),
		}})
		m = tm.(*Model)
	}

	tracker := &outputTracker{}
	currentTracker = tracker
	defer func() { currentTracker = nil }()

	// 2. End Tool B (Middle)
	tm, cmd := m.Update(eventMsg{event: domain.ToolEndEvent{CallID: "B"}})
	m = tm.(*Model)
	tracker.capture(cmd)

	// ASSERT: Tool B must NOT be in history yet (blocked by A)
	assert.NotContains(t, tracker.allHistory(), "Tool B")
	assert.Contains(t, m.View(), "Tool B") // Should still be in status area

	// 3. End Tool C (Last)
	tm, cmd = m.Update(eventMsg{event: domain.ToolEndEvent{CallID: "C"}})
	m = tm.(*Model)
	tracker.capture(cmd)

	// ASSERT: Tool C must NOT be in history yet (blocked by A)
	assert.NotContains(t, tracker.allHistory(), "Tool C")

	// 4. End Tool A (Head) -> Should trigger cascading graduation
	tm, cmd = m.Update(eventMsg{event: domain.ToolEndEvent{CallID: "A"}})
	m = tm.(*Model)
	tracker.capture(cmd)

	// ASSERT: All graduated in order A -> B -> C
	history := tracker.allHistory()
	idxA := strings.Index(history, "Tool A")
	idxB := strings.Index(history, "Tool B")
	idxC := strings.Index(history, "Tool C")

	assert.NotEqual(t, -1, idxA)
	assert.NotEqual(t, -1, idxB)
	assert.NotEqual(t, -1, idxC)
	assert.True(t, idxA < idxB, "A must be flushed before B")
	assert.True(t, idxB < idxC, "B must be flushed before C")

	// ASSERT: Status area empty
	assert.Empty(t, m.activeTools)
	assert.Empty(t, m.toolOrder)
}

func (o *outputTracker) allHistory() string {
	return strings.Join(o.history, "|")
}

func TestModel_Update_UnrollTextEvent_QueueSplitting(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewTestModel(events)

	// Send a BigTextEvent
	msg := eventMsg{event: domain.TextEvent{Text: "1234567890"}} // 10 chars

	// Process it. It should process "1234", and leave "5678" and "90" as separate TextEvents in the queue.
	m2, cmd := m.Update(msg)
	m = m2.(*Model)

	assert.NotNil(t, cmd, "Should return a command (stream tick)")
	assert.Len(t, m.queue, 2, "Queue should contain exactly 2 remaining items")

	te1, ok := m.queue[0].(domain.TextEvent)
	assert.True(t, ok, "First remaining item should be a TextEvent")
	assert.Equal(t, "5678", te1.Text, "First remaining item should be next 4 runes")

	te2, ok := m.queue[1].(domain.TextEvent)
	assert.True(t, ok, "Second remaining item should be a TextEvent")
	assert.Equal(t, "90", te2.Text, "Second remaining item should be the last 2 runes")
}

func TestModel_Update_WindowSize_NoResize(t *testing.T) {
	events := make(chan domain.Event)
	cfg := config.DefaultConfig().UI
	cfg.ChatWindowWidth = 80

	m := NewModel(events, cfg)
	initialWidth := m.width
	initialHeight := m.height

	// Simulate window resize to something much larger
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 100})

	assert.Equal(t, initialWidth, m.width, "Width should not change on WindowSizeMsg")
	assert.Equal(t, initialHeight, m.height, "Height should not change on WindowSizeMsg")
}

func TestModel_RenderTool_WidthConstraint(t *testing.T) {
	events := make(chan domain.Event)
	cfg := config.DefaultConfig().UI
	cfg.ChatWindowWidth = 80

	m := NewModel(events, cfg)

	ts := &toolState{
		id:      "test",
		display: domain.StringDisplay("hello"),
		status:  ui.StatusSuccess,
	}

	rendered := m.renderTool(ts)
	lines := strings.SplitSeq(rendered, "\n")

	for line := range lines {
		// Strip ANSI codes before measuring width
		width := lipgloss.Width(line)
		if width > 0 {
			assert.Equal(t, 80, width, "Each line of the tool box should be exactly 80 characters wide")
		}
	}
}

func TestThinking_Colors(t *testing.T) {
	origProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(origProfile)

	events := make(chan domain.Event, 10)
	m := NewTestModel(events)
	tracker := &outputTracker{}
	currentTracker = tracker
	defer func() { currentTracker = nil }()

	// 1. Running state
	m.isThinking = true
	m.thinkStart = time.Now().Add(-5 * time.Second)
	m.thinkingDuration = 5 * time.Second
	m.spinner.Spinner.Frames = []string{"X"} // Stable frame for testing

	view := m.View()
	// Should contain the styled duration
	styleRunning := lipgloss.NewStyle().Foreground(m.theme.PrimaryColor())
	assert.Contains(t, view, styleRunning.Render("Thinking for 5s"))

	// 2. Success state
	// Sending TextEvent should trigger thinking completion
	tm, cmd := m.Update(eventMsg{event: domain.TextEvent{Text: "done"}})
	m = tm.(*Model)
	tracker.capture(cmd)

	history := tracker.allHistory()
	assert.Contains(t, history, "✔")
	style := lipgloss.NewStyle().Foreground(m.theme.SuccessColor())
	assert.Contains(t, history, style.Render("Thought for 5s"))
}

func TestThinking_Cancelled(t *testing.T) {
	origProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(origProfile)

	events := make(chan domain.Event, 10)
	m := NewTestModel(events)
	tracker := &outputTracker{}
	currentTracker = tracker
	defer func() { currentTracker = nil }()

	m.isThinking = true
	m.thinkStart = time.Now().Add(-5 * time.Second)
	m.thinkingDuration = 5 * time.Second

	// Trigger Ctrl+C
	tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = tm.(*Model)
	tracker.capture(cmd)

	history := tracker.allHistory()
	// Should contain red cross and "Thought for 5s"
	assert.Contains(t, history, "✘", "Cancelled thinking should show a cross")
	style := lipgloss.NewStyle().Foreground(m.theme.ErrorColor())
	assert.Contains(t, history, style.Render("Thought for 5s"))
}
