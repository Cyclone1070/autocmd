package main

import (
	"fmt"
	"os"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	events := make(chan domain.Event)
	m := ui.NewModel(events, config.DefaultConfig().UI)

	p := tea.NewProgram(m)

	go func() {
		runSuite := func(name string, display1, display2, display3 domain.ToolDisplay) {
			events <- domain.TextEvent{Text: fmt.Sprintf("\n### SUITE: %s\n", name)}

			// Start all three
			events <- domain.ToolStartEvent{CallID: name + "-1", Display: display1}
			events <- domain.ToolStartEvent{CallID: name + "-2", Display: display2}
			events <- domain.ToolStartEvent{CallID: name + "-3", Display: display3}

			// Finishes: 2 (0.5s), 3 (1.5s), 1 (3s)
			time.Sleep(500 * time.Millisecond)
			events <- domain.ToolEndEvent{CallID: name + "-2"}

			time.Sleep(1000 * time.Millisecond)
			events <- domain.ToolEndEvent{CallID: name + "-3"}

			time.Sleep(1500 * time.Millisecond)
			events <- domain.ToolEndEvent{CallID: name + "-1"}
		}

		// 1. String Suite
		runSuite("STRING",
			domain.StringDisplay("String 1 (Slow)"),
			domain.StringDisplay("String 2 (Fast)"),
			domain.StringDisplay("String 3 (Medium)"))

		// 2. Diff Suite
		runSuite("DIFF",
			domain.DiffDisplay{Header: "file.txt", Diff: "- old\n+ new"},
			domain.DiffDisplay{Header: "fast.txt", Diff: "- fast\n+ gone"},
			domain.DiffDisplay{Header: "med.txt", Diff: "- meh\n+ okay"})

		// 3. Shell Suite (with streaming)
		events <- domain.TextEvent{Text: "\n### SUITE: SHELL (with streaming)\n"}
		events <- domain.ToolStartEvent{CallID: "SHELL-1", Display: domain.ShellDisplay{Command: "slow-cmd", Header: "Slow Shell"}}
		events <- domain.ToolStartEvent{CallID: "SHELL-2", Display: domain.ShellDisplay{Command: "fast-cmd", Header: "Fast Shell"}}
		events <- domain.ToolStartEvent{CallID: "SHELL-3", Display: domain.ShellDisplay{Command: "med-cmd", Header: "Medium Shell"}}

		// Stream some output
		for i := 0; i < 5; i++ {
			events <- domain.ToolStreamEvent{CallID: "SHELL-1", Chunk: fmt.Sprintf("Slow line %d...\n", i)}
			events <- domain.ToolStreamEvent{CallID: "SHELL-2", Chunk: fmt.Sprintf("Fast line %d...\n", i)}
			events <- domain.ToolStreamEvent{CallID: "SHELL-3", Chunk: fmt.Sprintf("Med line %d...\n", i)}
			time.Sleep(100 * time.Millisecond)
		}

		// Finishes
		events <- domain.ToolEndEvent{CallID: "SHELL-2"} // Fast
		time.Sleep(1000 * time.Millisecond)
		events <- domain.ToolEndEvent{CallID: "SHELL-3"} // Medium
		time.Sleep(1000 * time.Millisecond)
		events <- domain.ToolEndEvent{CallID: "SHELL-1"} // Slow

		events <- domain.DoneEvent{}
	}()

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
