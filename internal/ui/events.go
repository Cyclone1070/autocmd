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
		flushedBlocks, err := m.streamingMd.Append(e.Text)
		if err != nil {
			// Log error but continue?
			cmds = append(cmds, m.schedulePrint(fmt.Sprintf("Error rendering markdown: %v", err)))
		}

		for _, block := range flushedBlocks {
			cmds = append(cmds, m.schedulePrint(block))

			// Reduce maxContentHeight as content is flushed to history
			lines := strings.Count(block, "\n") + 1
			m.maxContentHeight -= lines
		}

		if m.maxContentHeight < 0 {
			m.maxContentHeight = 0
		}

		if len(cmds) > 0 {
			return m, tea.Sequence(cmds...)
		}
		return m, nil

	case domain.ToolStartEvent:
		m.thinking = false

		// Flush completed tools first (heuristic: new tool implies old ones might be done done)
		var cmds []tea.Cmd
		cmds = append(cmds, m.flushCompletedTools()...)

		// Note: We do NOT force flush text here. We rely on StreamingMarkdown logic.
		// If the text block was "Uncertain", it remains "Uncertain" until a new block starts
		// or we force flush.

		textFlush, _ := m.streamingMd.Flush()
		if textFlush != "" {
			cmds = append(cmds, m.schedulePrint(textFlush))

			// Reduce maxContentHeight
			lines := strings.Count(textFlush, "\n") + 1
			m.maxContentHeight -= lines
			if m.maxContentHeight < 0 {
				m.maxContentHeight = 0
			}
		}

		// Initialize new tool
		ts := &toolState{
			callID:  e.CallID,
			display: e.Display,
			status:  statusRunning,
		}
		m.tools = append(m.tools, ts)

		cmds = append(cmds, m.spinner.Tick)
		return m, tea.Sequence(cmds...)

	case domain.ToolStreamEvent:
		// Find tool in slice
		for _, ts := range m.tools {
			if ts.callID == e.CallID {
				ts.shellOutput.WriteString(e.Chunk)
				break
			}
		}
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
	textFlush, _ := m.streamingMd.Flush()
	if textFlush != "" {
		cmds = append(cmds, m.schedulePrint(strings.TrimRight(textFlush, "\n")))
	}

	// 2. Flush remaining tools
	for _, ts := range m.tools {
		cmds = append(cmds, m.schedulePrint(m.viewTool(ts)))
	}
	m.tools = nil // Clear state

	// 3. Queue FINAL Status Bar print
	// We want this to be the very last thing, so we queue it.
	// The PrintQueue guarantees it appears AFTER the above flushes.
	finalStatus := strings.TrimPrefix(m.statusBar(), "\n")
	cmds = append(cmds, m.schedulePrint(finalStatus))

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
		cmds = append(cmds, m.schedulePrint(output))

		// Adjust maxContentHeight downwards because this content is moving to history
		// We count lines in the output (plus 2 for the double-newline join separation that would have been there)
		lines := strings.Count(output, "\n") + 1

		// Also account for the separation that View() adds between tools (\n\n)
		// If there were multiple tools, each had separation.
		// Use a heuristic: just decrement by the tool height for now.
		if m.maxContentHeight > lines {
			m.maxContentHeight -= lines
		} else {
			m.maxContentHeight = 0
		}
	}

	return cmds
}
