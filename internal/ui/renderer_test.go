package ui

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

// Helper to create model for testing
func newTestModel(t *testing.T) *model {
	t.Helper()
	m, err := newModel(config.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	// For testing, mock terminal size to something standard
	m.width = 80
	m.termHeight = 24
	return m
}

func TestModel_ThinkingEvent(t *testing.T) {
	m := newTestModel(t)

	// Send ThinkingEvent
	updatedM, cmd := m.Update(msg{Event: domain.ThinkingEvent{}})

	// Check state
	newM := updatedM.(*model)
	assert.True(t, newM.thinking)
	assert.NotNil(t, cmd) // Should trigger tick

	// Check View
	view := newM.View()
	assert.Contains(t, view, "Thinking")

	// Reset
	m = newTestModel(t)
	m.Update(msg{Event: domain.TextEvent{Text: "Done"}})
	newM = m
	assert.False(t, newM.thinking)
}

func TestModel_TextEvent(t *testing.T) {
	m := newTestModel(t)

	// Send TextEvent
	updatedM, _ := m.Update(msg{Event: domain.TextEvent{Text: "**Bold** text"}})
	newM := updatedM.(*model)

	assert.False(t, newM.thinking)
	// StreamingMarkdown pending content should be visible
	assert.Contains(t, newM.View(), "Bold")
	assert.NotEmpty(t, newM.streamingMd.Pending())

	// Flush it by starting a tool
	// Note: In new algo, TextEvent appends are NOT automatically flushed until
	// 1. A complete block is formed AND another block starts
	// 2. OR explicit flush call
	// But ToolStart forces a flush of pending text.

	startEv := domain.ToolStartEvent{CallID: "c1", ToolName: "t1"}
	updatedM, _ = newM.Update(msg{Event: startEv})
	newM = updatedM.(*model)

	// After tool start, pending text should be flushed (empty buffer)
	assert.Equal(t, "", newM.streamingMd.Pending())
}

func TestModel_ToolEvents_StringDisplay(t *testing.T) {
	m := newTestModel(t)

	// 1. ToolStart
	startEv := domain.ToolStartEvent{
		CallID:   "call-1",
		ToolName: "read_file",
		Display:  domain.StringDisplay("Reading file.txt"),
	}
	updatedM, _ := m.Update(msg{Event: startEv})
	newM := updatedM.(*model)

	// Check tools slice
	assert.Len(t, newM.tools, 1)
	assert.Equal(t, "call-1", newM.tools[0].callID)
	assert.Equal(t, statusRunning, newM.tools[0].status)

	assert.Contains(t, newM.View(), "Reading file.txt")
	assert.Contains(t, newM.View(), "⣾") // partial check for spinner stuff

	// 2. ToolEnd (Success)
	endEv := domain.ToolEndEvent{CallID: "call-1"}
	updatedM, _ = newM.Update(msg{Event: endEv})
	newM = updatedM.(*model)

	// In new model, completed tools stay in slice until flushed.
	// But if it's the head of the queue, it tries to flush immediately on ToolEnd.
	// Since we can't capture tea.Println output easily here,
	// we just check that the tool was processed.

	// If flushed successfully, it should be removed from m.tools
	assert.Len(t, newM.tools, 0)

	// 3. ToolEnd (Error) - Setup new scenario with error
	m = newTestModel(t)
	m.Update(msg{Event: startEv})
	newM = m

	errEv := domain.ToolEndEvent{CallID: "call-1", Error: "permission denied"}
	updatedM, _ = newM.Update(msg{Event: errEv})
	newM = updatedM.(*model)

	// Should also flush immediately because it's first in line
	assert.Len(t, newM.tools, 0)
}

func TestModel_ToolEvents_ShellDisplay(t *testing.T) {
	m := newTestModel(t)

	// 1. ToolStart
	startEv := domain.ToolStartEvent{
		CallID:   "call-shell",
		ToolName: "shell",
		Display: domain.ShellDisplay{
			Header:  "Running ls",
			Command: "ls -la",
		},
	}
	updatedM, _ := m.Update(msg{Event: startEv})
	newM := updatedM.(*model)

	assert.Contains(t, newM.View(), "$ ls -la")
	assert.Contains(t, newM.View(), "Running ls")

	// 2. Stream
	streamEv := domain.ToolStreamEvent{
		CallID: "call-shell",
		Chunk:  "file1\n",
	}
	updatedM, _ = newM.Update(msg{Event: streamEv})
	newM = updatedM.(*model)

	assert.Contains(t, newM.View(), "file1")

	// 3. End
	endEv := domain.ToolEndEvent{CallID: "call-shell"}
	updatedM, _ = newM.Update(msg{Event: endEv})
	newM = updatedM.(*model)

	// Should be flushed
	assert.Len(t, newM.tools, 0)
}

func TestModel_ConcurrentEvents_HandledSequentially(t *testing.T) {
	// Bubble Tea processes sequentially in Update loop, so we assume concurrency safety handled by tea.Program
	// We just test logic transitions
	m := newTestModel(t)

	m.Update(msg{Event: domain.ThinkingEvent{}})
	newM := m

	assert.True(t, newM.thinking)
}

func TestModel_Init(t *testing.T) {
	m := newTestModel(t)
	cmd := m.Init()
	assert.NotNil(t, cmd)
}

func TestModel_CtrlC(t *testing.T) {
	m := newTestModel(t)
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	assert.NotNil(t, cmd)
	assert.Equal(t, stateCancelled, newM.(*model).runState)
}

func TestModel_DoneEvent(t *testing.T) {
	// Case 1: Done with no pending text -> Quit
	m := newTestModel(t)
	_, cmd := m.Update(msg{Event: domain.DoneEvent{}})
	// Should be sequence containing Quit
	assert.NotNil(t, cmd)

	// Case 2: Done with pending text -> Render, Print, Quit
	m = newTestModel(t)
	m.Update(msg{Event: domain.TextEvent{Text: "Final Text"}})

	newM, cmd := m.Update(msg{Event: domain.DoneEvent{}})

	assert.NotNil(t, cmd)
	// Pending text should be gone (flushed)
	assert.Equal(t, "", newM.(*model).streamingMd.Pending())
	assert.Equal(t, stateDone, newM.(*model).runState)
}

func TestModel_OrderedFlushing(t *testing.T) {
	m := newTestModel(t)

	// Start Tool A
	m.Update(msg{Event: domain.ToolStartEvent{CallID: "A", ToolName: "A", Display: domain.StringDisplay("A")}})

	// Start Tool B
	m.Update(msg{Event: domain.ToolStartEvent{CallID: "B", ToolName: "B", Display: domain.StringDisplay("B")}})

	newM := m
	assert.Len(t, newM.tools, 2)
	assert.Equal(t, "A", newM.tools[0].callID)
	assert.Equal(t, "B", newM.tools[1].callID)

	// Finish Tool B (should NOT flush because A is running)
	updatedM, _ := m.Update(msg{Event: domain.ToolEndEvent{CallID: "B"}})
	newM = updatedM.(*model)

	assert.Len(t, newM.tools, 2)
	assert.Equal(t, statusSuccess, newM.tools[1].status) // B is done
	assert.Equal(t, statusRunning, newM.tools[0].status) // A is running

	// Finish Tool A (should flush A, then B)
	updatedM, _ = m.Update(msg{Event: domain.ToolEndEvent{CallID: "A"}})
	newM = updatedM.(*model)

	assert.Len(t, newM.tools, 0) // Both flushed
}
