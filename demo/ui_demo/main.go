package main

import (
	"fmt"
	"os"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui/loop"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	events := make(chan domain.Event)
	m := loop.NewModel(events, config.DefaultConfig().UI)

	p := tea.NewProgram(m)

	// Simulate workflow in background
	go func() {
		// 1. Text Stream
		markdown := "# UI Demo\n\nThis is a demo of the **smooth streaming** logic. It breaks down text into small chunks to simulate a real-time LLM response.\n\n"
		events <- domain.TextEvent{Text: markdown}
		time.Sleep(400 * time.Millisecond)

		// 2. Thinking
		events <- domain.ThinkingEvent{}
		time.Sleep(1 * time.Second)

		events <- domain.TextEvent{Text: "Here's a readfile tool call."}

		events <- domain.ThinkingEvent{}
		time.Sleep(1 * time.Second)

		events <- domain.ToolStartEvent{
			CallID:  "tool-0",
			Display: domain.NewStringDisplay("Reading main.go"),
		}
		time.Sleep(1 * time.Second)
		events <- domain.ToolEndEvent{CallID: "tool-0"}

		events <- domain.TextEvent{Text: "Now let's run some tools in parallel. Tools will be displayed in toolStart order."}
		time.Sleep(400 * time.Millisecond)

		// 3. Parallel Tool Calls (3 tools)
		events <- domain.ToolStartEvent{
			CallID: "tool-1",
			Display: domain.ShellDisplay{
				Header:  "Finish last",
				Command: "npm list --depth=0",
			},
		}
		time.Sleep(400 * time.Millisecond)

		events <- domain.ToolStartEvent{
			CallID: "tool-2",
			Display: domain.ShellDisplay{
				Header:  "Finish first",
				Command: "eslint .",
			},
		}
		time.Sleep(200 * time.Millisecond)
		events <- domain.ToolStreamEvent{CallID: "tool-2", Chunk: "All files passed linting.\n"}
		time.Sleep(400 * time.Millisecond)

		events <- domain.ToolStartEvent{
			CallID: "tool-3",
			Display: domain.ShellDisplay{
				Header:  "Finish second",
				Command: "go test ./...",
			},
		}
		time.Sleep(400 * time.Millisecond)

		// 3a. Tool 2 finishes first (blocked)
		events <- domain.ToolStreamEvent{CallID: "tool-2", Chunk: "All files passed linting.\n"}
		time.Sleep(400 * time.Millisecond)
		events <- domain.ToolEndEvent{CallID: "tool-2"}
		time.Sleep(1 * time.Second)

		// 3b. Tool 3 finishes second (blocked)
		events <- domain.ToolStreamEvent{CallID: "tool-3", Chunk: "Running tests...\nPASS\n"}
		time.Sleep(400 * time.Millisecond)
		events <- domain.ToolEndEvent{CallID: "tool-3"}
		time.Sleep(1 * time.Second)

		// 3c. Tool 1 finishes last (triggers cascading flush)
		events <- domain.ToolStreamEvent{CallID: "tool-1", Chunk: "Found 45 dependencies.\n"}
		time.Sleep(400 * time.Millisecond)
		events <- domain.ToolEndEvent{CallID: "tool-1"}

		// 6. Final text
		events <- domain.TextEvent{Text: "\n\nRefactoring complete! The UI is looking great. ✨\n"}

		// 7. Done
		events <- domain.DoneEvent{}
	}()

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
