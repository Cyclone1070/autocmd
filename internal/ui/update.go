package ui

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) handleEvent(ev domain.Event) (tea.Model, tea.Cmd) {
	switch e := ev.(type) {
	case domain.ThinkingEvent:
		m.thinking = true
		return m, m.spinner.Tick

	case domain.TextEvent:
		m.thinking = false

		// Flush any completed tools at the front of the queue
		cmds := m.flushCompletedTools()

		// Append text and get flushable blocks
		flushedBlocks, err := m.streamingMd.append(e.Text)
		if err != nil {
			// Rendering failed. This is catastrophic. Clean shutdown.
			return m, tea.Sequence(
				m.schedulePrint(fmt.Sprintf("\nFatal: markdown rendering failed: %v", err)),
				tea.Quit,
			)
		}

		for _, block := range flushedBlocks {
			cmds = append(cmds, m.flushContent(block))
		}

		if m.maxContentHeight < 0 {
			m.maxContentHeight = 0
		}

		if len(cmds) > 0 {
			return m, tea.Sequence(cmds...)
		}
		m.updateMaxContentHeight()
		return m, nil

	case domain.ToolStartEvent:
		m.thinking = false

		// Flush completed tools first (heuristic: new tool implies old ones might be done done)
		var cmds []tea.Cmd
		cmds = append(cmds, m.flushCompletedTools()...)

		// Note: We do NOT force flush text here. We rely on StreamingMarkdown logic.
		// If the text block was "Uncertain", it remains "Uncertain" until a new block starts
		// or we force flush.

		textFlush, err := m.streamingMd.RenderRemaining()
		if err != nil {
			return m, tea.Sequence(
				m.schedulePrint(fmt.Sprintf("\nFatal: markdown flushing failed: %v", err)),
				tea.Quit,
			)
		}
		if textFlush != "" {
			cmds = append(cmds, m.flushContent(textFlush))
		}

		// Initialize new tool
		ts := &toolState{
			callID:  e.CallID,
			display: e.Display,
			status:  statusRunning,
		}
		m.tools = append(m.tools, ts)

		cmds = append(cmds, m.spinner.Tick)
		m.updateMaxContentHeight()
		return m, tea.Sequence(cmds...)

	case domain.ToolStreamEvent:
		// Find tool in slice
		for _, ts := range m.tools {
			if ts.callID == e.CallID {
				ts.shellOutput.WriteString(e.Chunk)
				break
			}
		}
		m.updateMaxContentHeight()
		return m, nil

	case domain.ToolEndEvent:
		// Mark tool as done
		for _, ts := range m.tools {
			if ts.callID == e.CallID {
				if e.Error != "" {
					ts.status = statusError
					ts.err = e.Error
				} else {
					ts.status = statusSuccess
				}
				break
			}
		}

		// Attempt flush from front
		cmds := m.flushCompletedTools()
		if len(cmds) > 0 {
			return m, tea.Sequence(cmds...)
		}
		m.updateMaxContentHeight()
		return m, nil

	case domain.DoneEvent:
		m.runState = stateDone // Start explicit wait, but show DONE status
		return m.handleDoneEvent()
	}

	return m, nil
}

// handleDoneEvent performs final flushes and triggers the safe exit wait logic.
func (m *model) handleDoneEvent() (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// 1. Flush pending text
	textFlush, err := m.streamingMd.RenderRemaining()
	if err != nil {
		cmds = append(cmds, m.schedulePrint(fmt.Sprintf("\nFatal: markdown flushing failed: %v", err)))
		// We are already quitting (handleDoneEvent is part of shutdown or leads to it),
		// but let's ensure the error is seen.
	} else if textFlush != "" {
		cmds = append(cmds, m.flushContent(strings.TrimRight(textFlush, "\n")))
	}

	// 2. Flush remaining tools
	for _, ts := range m.tools {
		cmds = append(cmds, m.flushContent(m.viewTool(ts)))
	}
	m.tools = nil // Clear state

	// 3. Queue FINAL Status Bar print
	// We want this to be the very last thing, so we queue it.
	// The PrintQueue guarantees it appears AFTER the above flushes.
	finalStatus := strings.TrimPrefix(m.statusBar(), "\n")
	cmds = append(cmds, m.flushContent(finalStatus))

	return m, tea.Sequence(cmds...)
}

// flushCompletedTools checks the front of the tool queue.
// If the first tool is complete, it flushes it and repeats for subsequent tools.
// Returns a list of tea.Println commands.
func (m *model) flushCompletedTools() []tea.Cmd {
	var cmds []tea.Cmd

	// While list is not empty AND first item is not running
	for len(m.tools) > 0 && m.tools[0].status != statusRunning {
		// Pop front
		tool := m.tools[0]
		m.tools = m.tools[1:]

		// Create flush command using tracked helper
		output := m.viewTool(tool)
		cmds = append(cmds, m.flushContent(output))
	}

	return cmds
}

// updateMaxContentHeight calculates the visual height of the current content
// and updates the model's maxContentHeight if it has grown.
// This must be called in Update() whenever content changes to ensure padding consistency.
func (m *model) updateMaxContentHeight() {
	content := m.renderContent()
	if content == "" {
		return
	}
	// Strictly count newlines for height
	// "hello" (0 newlines) occupies 1 visual line but 0 vertical lines relative to start.
	// "hello\n" (1 newline) occupies 1 vertical line.
	currentHeight := strings.Count(content, "\n")

	if currentHeight > m.maxContentHeight {
		m.maxContentHeight = currentHeight
	}
}

// flushContent atomically handles the transition of content from "Active/Pending" to "History".
//
// 1. It calculates the vertical height of the content.
// 2. It decrements m.maxContentHeight by that amount (clamped to 0).
// 3. It returns a tea.Cmd to print the content to the release queue.
func (m *model) flushContent(content string) tea.Cmd {
	if content == "" {
		return nil
	}

	// Strictly count newlines for height reduction
	// We add 1 because a string with N newlines occupies N+1 lines
	lines := strings.Count(content, "\n") + 1

	if m.maxContentHeight > lines {
		m.maxContentHeight -= lines
	} else {
		m.maxContentHeight = 0
	}

	return m.schedulePrint(content)
}
