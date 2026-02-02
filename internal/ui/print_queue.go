package ui

import tea "github.com/charmbracelet/bubbletea"

// msgPrintFinished is sent when a scheduled print command completes.
type msgPrintFinished struct{}

// schedulePrint adds content to the queue and attempts to process it.
func (m *model) schedulePrint(content string) tea.Cmd {
	if content == "" {
		return nil
	}
	m.printQueue = append(m.printQueue, content)
	return m.processQueue()
}

// processQueue checks if we can print the next item.
func (m *model) processQueue() tea.Cmd {
	// If already printing or nothing to print, wait.
	if m.isPrinting || len(m.printQueue) == 0 {
		return nil
	}

	// Pop
	content := m.printQueue[0]
	m.printQueue = m.printQueue[1:]

	// Lock
	m.isPrinting = true

	// Exec
	return tea.Sequence(
		tea.Println(content),
		func() tea.Msg { return msgPrintFinished{} },
	)
}
