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
			// Tool 3 is the "middle" finisher - make it fail.
			time.Sleep(500 * time.Millisecond)
			events <- domain.ToolEndEvent{CallID: name + "-2"}

			time.Sleep(1000 * time.Millisecond)
			events <- domain.ToolEndEvent{CallID: name + "-3", Error: "operation failed: middle tool error"}

			time.Sleep(1500 * time.Millisecond)
			events <- domain.ToolEndEvent{CallID: name + "-1"}
		}

		// 1. String Suite
		runSuite("STRING",
			domain.StringDisplay("String 1 (Slow)"),
			domain.StringDisplay("String 2 (Fast)"),
			domain.StringDisplay("String 3 (Medium/Fail)"))

		// 2. Diff Suite
		runSuite("DIFF",
			domain.DiffDisplay{Header: "file.txt", Diff: "- old\n+ new"},
			domain.DiffDisplay{Header: "fast.txt", Diff: "- fast\n+ gone"},
			domain.DiffDisplay{Header: "med.txt", Diff: "- error here\n+ failed"})

		// 3. Shell Suite (with more streaming)
		events <- domain.TextEvent{Text: "\n### SUITE: SHELL (Heavy Streaming)\n"}
		events <- domain.ToolStartEvent{CallID: "SHELL-1", Display: domain.ShellDisplay{Command: "slow-cmd", Header: "Slow Shell"}}
		events <- domain.ToolStartEvent{CallID: "SHELL-2", Display: domain.ShellDisplay{Command: "fast-cmd", Header: "Fast Shell"}}
		events <- domain.ToolStartEvent{CallID: "SHELL-3", Display: domain.ShellDisplay{Command: "med-cmd", Header: "Medium Shell (Fail)"}}

		// Heavy streaming
		for i := 1; i <= 20; i++ {
			if i <= 15 {
				events <- domain.ToolStreamEvent{CallID: "SHELL-2", Chunk: fmt.Sprintf("Fast output line %d - working quickly...\n", i)}
			}
			if i <= 20 {
				events <- domain.ToolStreamEvent{CallID: "SHELL-1", Chunk: fmt.Sprintf("Slow output line %d - taking its time...\n", i)}
			}
			if i <= 18 {
				events <- domain.ToolStreamEvent{CallID: "SHELL-3", Chunk: fmt.Sprintf("Med output line %d - about to crash...\n", i)}
			}
			time.Sleep(300 * time.Millisecond)
		}

		// Finishes
		events <- domain.ToolEndEvent{CallID: "SHELL-2"} // Fast
		time.Sleep(1000 * time.Millisecond)
		events <- domain.ToolEndEvent{CallID: "SHELL-3", Error: "exit status 1: middle tool error"} // Medium/Fail
		time.Sleep(1000 * time.Millisecond)
		events <- domain.ToolEndEvent{CallID: "SHELL-1"} // Slow

		events <- domain.DoneEvent{}
	}()

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
