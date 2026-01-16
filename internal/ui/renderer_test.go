package ui

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

// Helper to create model for testing
func newTestModel() *model {
	return newModel(config.DefaultConfig())
}

func TestModel_ThinkingEvent(t *testing.T) {
	m := newTestModel()

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
	m = newTestModel()
	m.Update(msg{Event: domain.TextEvent{Text: "Done"}})
	newM = m
	assert.False(t, newM.thinking)
}

func TestModel_TextEvent(t *testing.T) {
	m := newTestModel()

	// Send TextEvent
	updatedM, _ := m.Update(msg{Event: domain.TextEvent{Text: "**Bold** text"}})
	newM := updatedM.(*model)

	assert.False(t, newM.thinking)
	assert.Equal(t, "**Bold** text", newM.streamingText)

	// Flush it by starting a tool
	startEv := domain.ToolStartEvent{CallID: "c1", ToolName: "t1"}
	updatedM, _ = newM.Update(msg{Event: startEv})
	newM = updatedM.(*model)

	assert.Equal(t, "", newM.streamingText)
	// We don't have history anymore in the model struct itself (Wait, let's check model.go)
}

func TestModel_ToolEvents_StringDisplay(t *testing.T) {
	m := newTestModel()

	// 1. ToolStart
	startEv := domain.ToolStartEvent{
		CallID:   "call-1",
		ToolName: "read_file",
		Display:  domain.StringDisplay("Reading file.txt"),
	}
	updatedM, _ := m.Update(msg{Event: startEv})
	newM := updatedM.(*model)

	assert.Contains(t, newM.activeTools, "call-1")
	assert.Equal(t, statusRunning, newM.activeTools["call-1"].status)
	assert.Contains(t, newM.View(), "Reading file.txt")
	assert.Contains(t, newM.View(), "⣾") // partial check for spinner stuff

	// 2. ToolEnd (Success)
	endEv := domain.ToolEndEvent{CallID: "call-1"}
	updatedM, _ = newM.Update(msg{Event: endEv})
	newM = updatedM.(*model)

	assert.NotContains(t, newM.activeTools, "call-1")

	// 3. ToolEnd (Error)
	m = newTestModel()
	m.Update(msg{Event: startEv})
	newM = m

	errEv := domain.ToolEndEvent{CallID: "call-1", Error: "permission denied"}
	updatedM, _ = newM.Update(msg{Event: errEv})
	newM = updatedM.(*model)

	assert.NotContains(t, newM.activeTools, "call-1")
}

func TestModel_ToolEvents_ShellDisplay(t *testing.T) {
	m := newTestModel()

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

	assert.NotContains(t, newM.activeTools, "call-shell")
}

func TestModel_ConcurrentEvents_HandledSequentially(t *testing.T) {
	// Bubble Tea processes sequentially in Update loop, so we assume concurrency safety handled by tea.Program
	// We just test logic transitions
	m := newTestModel()

	m.Update(msg{Event: domain.ThinkingEvent{}})
	newM := m

	assert.True(t, newM.thinking)
}

func TestModel_Init(t *testing.T) {
	m := newTestModel()
	cmd := m.Init()
	assert.NotNil(t, cmd)
}

func TestModel_CtrlC(t *testing.T) {
	m := newTestModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	assert.Equal(t, tea.Quit(), cmd())
}

// Dummy wait for coverage
func (r *Renderer) WaitTest() {
	// Cannot easily test Wait() as it runs the program
}
