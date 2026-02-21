package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
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

// dispatch recursively unpacks batches and feeds messages to Update.
// It stops when a message is waitForEvent or streamTickMsg to allow test assertions.
func dispatch(m *Model, cmd tea.Cmd) *Model {
	if cmd == nil {
		return m
	}
	msg := cmd()
	return handleMsg(m, msg)
}

func handleMsg(m *Model, msg tea.Msg) *Model {
	if msg == nil {
		return m
	}

	switch msg := msg.(type) {
	case tea.BatchMsg:
		for _, cmd := range msg {
			m = dispatch(m, cmd)
		}
		return m
	case eventMsg, streamTickMsg:
		tm, _ := m.Update(msg)
		return tm.(*Model)
	default:
		tm, _ := m.Update(msg)
		return tm.(*Model)
	}
}

func TestModel_Update_SmoothStreaming(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewModel(events, 80)
	renderer := &engineMockRenderer{}
	m.stream = NewStream(renderer)

	// Send a TextEvent with multiple chunks worth of characters
	longText := "12345678" // 8 chars = 2 chunks
	msg := eventMsg{event: domain.TextEvent{Text: longText}}

	// First update should not flush yet (it queues)
	tm, cmd := m.Update(msg)
	m = tm.(*Model)
	assert.Equal(t, longText, m.textQueue)
	assert.NotNil(t, cmd)

	// The cmd is a batch {streamTick, waitForEvent}.
	// We want to extract the streamTickMsg.
	mMsg := cmd()
	batch, ok := mMsg.(tea.BatchMsg)
	assert.True(t, ok)

	var tickMsg tea.Msg
	for _, bcmd := range batch {
		if bcmd == nil {
			continue
		}
		// Try to execute without blocking
		select {
		case events <- domain.DoneEvent{}: // Unblock waitForEvent if we hit it
		default:
		}
		bmsg := bcmd()
		if _, ok := bmsg.(streamTickMsg); ok {
			tickMsg = bmsg
			break
		}
	}
	assert.NotNil(t, tickMsg)

	// Process tick 1
	tm, cmd = m.Update(tickMsg)
	m = tm.(*Model)
	assert.Equal(t, "5678", m.textQueue)
	assert.Equal(t, "1234", m.stream.buffer)

	// Extract next tick from batch {flushHistory, streamTick, waitForEvent}
	mMsg = cmd()
	batch, ok = mMsg.(tea.BatchMsg)
	assert.True(t, ok)
	tickMsg = nil
	for _, bcmd := range batch {
		if bcmd == nil {
			continue
		}
		bmsg := bcmd()
		if _, ok := bmsg.(streamTickMsg); ok {
			tickMsg = bmsg
			break
		}
	}
	assert.NotNil(t, tickMsg)

	// Process tick 2
	tm, _ = m.Update(tickMsg)
	m = tm.(*Model)
	assert.Equal(t, "", m.textQueue)
	assert.Equal(t, "12345678", m.stream.buffer)
}

func TestModel_Update_TextEvent(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewModel(events, 80)
	renderer := &engineMockRenderer{}
	m.stream = NewStream(renderer)

	msg := eventMsg{event: domain.TextEvent{Text: "Hello\n\nWorld"}}
	_, cmd := m.Update(msg)

	assert.NotNil(t, cmd)
	// cmd should be a batch {streamTick, waitForEvent}
}

func TestModel_View_Truncation(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewModel(events, 80)
	m.height = 5

	longText := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10"
	renderer := &engineMockRenderer{}
	m.stream = NewStream(renderer)
	m.stream.Append(longText)

	view := m.View()

	lines := 0
	for _, char := range view {
		if char == '\n' {
			lines++
		}
	}
	assert.LessOrEqual(t, lines+1, m.height)
	assert.Contains(t, view, "▲ [Truncated]")
}

func TestModel_Update_ThinkingEvent(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewModel(events, 80)

	// 1. Send ThinkingEvent
	tm, cmd := m.Update(eventMsg{event: domain.ThinkingEvent{}})
	m = tm.(*Model)
	assert.True(t, m.isThinking)
	assert.NotZero(t, m.thinkStart)
	assert.NotNil(t, cmd)

	// 2. Send TextEvent to finish thinking
	msg := eventMsg{event: domain.TextEvent{Text: "Thinking done"}}
	tm, cmd = m.Update(msg)
	m = tm.(*Model)

	assert.False(t, m.isThinking)
	assert.NotZero(t, m.thinkEnd)
	assert.NotNil(t, cmd)

	// Concrete assertion on Printf content
	select {
	case events <- domain.DoneEvent{}:
	default:
	}
	mMsg := cmd()
	if batch, ok := mMsg.(tea.BatchMsg); ok {
		foundPrintf := false
		for _, bcmd := range batch {
			if bcmd == nil {
				continue
			}
			bmsg := bcmd()
			s := fmt.Sprintf("%v", bmsg)
			if strings.Contains(s, "Thought for") {
				foundPrintf = true
				break
			}
		}
		assert.True(t, foundPrintf, "Should have found Printf in batch")
	} else {
		s := fmt.Sprintf("%v", mMsg)
		assert.Contains(t, s, "Thought for")
	}
}

func TestModel_Update_EventLoopContinuity(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewModel(events, 80)

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

	// 2. TextEvent (start of streaming) must batch waitForEvent
	m = NewModel(events, 80)
	_, cmd = m.Update(eventMsg{event: domain.TextEvent{Text: "Hello"}})
	assert.NotNil(t, cmd)

	events <- domain.DoneEvent{}
	mMsg = cmd()

	batch, ok = mMsg.(tea.BatchMsg)
	assert.True(t, ok, "TextEvent should return a Batch to keep polling")
}

func TestModel_Update_KeyMsg_Quit(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewModel(events, 80)

	// Ctrl+C should quit
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	assert.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok, "Ctrl+C should return tea.QuitMsg")
}

func TestModel_Update_ToolLifecycle(t *testing.T) {
	events := make(chan domain.Event, 10)
	m := NewModel(events, 80)

	// 1. Tool Start
	startEv := domain.ToolStartEvent{
		CallID: "123",
		Display: domain.ShellDisplay{
			Header:  "Run tests",
			Command: "go test ./...",
		},
	}
	m2, _ := m.Update(eventMsg{event: startEv})
	m = m2.(*Model)

	assert.NotNil(t, m.activeTool)
	assert.Equal(t, "123", m.activeTool.id)

	// 2. Tool Stream
	streamEv := domain.ToolStreamEvent{
		CallID: "123",
		Chunk:  "PASS\n",
	}
	m2, _ = m.Update(eventMsg{event: streamEv})
	m = m2.(*Model)
	assert.Contains(t, m.activeTool.output, "PASS")

	// 3. Tool End
	endEv := domain.ToolEndEvent{
		CallID: "123",
		Error:  "",
	}
	m2, cmd := m.Update(eventMsg{event: endEv})
	m = m2.(*Model)

	assert.Nil(t, m.activeTool)
	assert.NotNil(t, cmd)
}
