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

	// Simulate workflow in background
	go func() {
		// 1. Thinking
		events <- domain.ThinkingEvent{}
		time.Sleep(1 * time.Second)

		// 2. Text Stream
		markdown := "# UI Demo\n\nThis is a demo of the **smooth streaming** logic. It breaks down text into small chunks to simulate a real-time LLM response.\n\n"
		events <- domain.TextEvent{Text: markdown}
		time.Sleep(1 * time.Second)

		// 3. Parallel Tool Calls
		events <- domain.ToolStartEvent{
			CallID: "slow-tool",
			Display: domain.ShellDisplay{
				Header:  "Slow Background Process",
				Command: "sleep 10",
			},
		}
		time.Sleep(500 * time.Millisecond)

		events <- domain.ToolStartEvent{
			CallID: "fast-tool",
			Display: domain.ShellDisplay{
				Header:  "Quick Status Check",
				Command: "ls -la",
			},
		}
		time.Sleep(500 * time.Millisecond)

		events <- domain.ToolStreamEvent{CallID: "slow-tool", Chunk: "Working...\n"}
		time.Sleep(500 * time.Millisecond)
		events <- domain.ToolStreamEvent{CallID: "fast-tool", Chunk: "total 123\ndrwxr-xr-x  2 user  group  64 ...\n"}
		time.Sleep(500 * time.Millisecond)

		// Fast tool finishes first
		events <- domain.ToolEndEvent{CallID: "fast-tool"}
		time.Sleep(1 * time.Second)

		// Slow tool finishes last
		events <- domain.ToolEndEvent{CallID: "slow-tool"}

		// 6. Final text
		events <- domain.TextEvent{Text: "\n\nRefactoring complete! The UI is looking great. ✨\n"}
		time.Sleep(1 * time.Second)

		// 7. Done
		events <- domain.DoneEvent{}
	}()

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
