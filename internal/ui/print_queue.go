package ui

import tea "github.com/charmbracelet/bubbletea"

// printItem represents a content piece to be printed
type printItem struct {
	content string
	raw     bool // If true, use tea.Print (no newline); else tea.Println
}

// msgPrintFinished is sent when a scheduled print command completes.
type msgPrintFinished struct{}

// schedulePrint adds content to the queue and attempts to process it (standard Println).
func (m *model) schedulePrint(content string) tea.Cmd {
	if content == "" {
		return nil
	}
	m.printQueue = append(m.printQueue, printItem{content: content, raw: false})
	return m.processQueue()
}

// schedulePrintRaw adds content to the queue for raw printing (tea.Print).
func (m *model) schedulePrintRaw(content string) tea.Cmd {
	if content == "" {
		return nil
	}
	m.printQueue = append(m.printQueue, printItem{content: content, raw: true})
	return m.processQueue()
}

// processQueue checks if we can print the next item.
func (m *model) processQueue() tea.Cmd {
	// If already printing or nothing to print, wait.
	if m.isPrinting || len(m.printQueue) == 0 {
		return nil
	}

	// Pop
	item := m.printQueue[0]
	m.printQueue = m.printQueue[1:]

	// Lock
	m.isPrinting = true

	// Exec
	var printCmd tea.Cmd
	if item.raw {
		printCmd = tea.Printf("%s", item.content)
	} else {
		printCmd = tea.Println(item.content)
	}

	return tea.Sequence(
		printCmd,
		func() tea.Msg { return msgPrintFinished{} },
	)
}
