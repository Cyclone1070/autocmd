package main

import (
	"fmt"
	"os"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	events := make(chan domain.Event)
	m := ui.NewModel(events, 80)

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

		// 3. Tool Call: Shell
		events <- domain.ToolStartEvent{
			CallID: "shell-1",
			Display: domain.ShellDisplay{
				Header:  "Running tests",
				Command: "go test -v ./internal/ui/...",
			},
		}
		time.Sleep(1 * time.Second)
		events <- domain.ToolStreamEvent{CallID: "shell-1", Chunk: "=== RUN   TestStream\n"}
		time.Sleep(500 * time.Millisecond)
		events <- domain.ToolStreamEvent{CallID: "shell-1", Chunk: "--- PASS: TestStream (0.50s)\n"}
		time.Sleep(500 * time.Millisecond)
		events <- domain.ToolEndEvent{CallID: "shell-1"}

		// 4. Thinking again
		events <- domain.ThinkingEvent{}
		time.Sleep(2 * time.Second)

		// 5. Tool Call: Diff
		events <- domain.ToolStartEvent{
			CallID: "diff-1",
			Display: domain.DiffDisplay{
				Header:  "Updating main.go",
				Diff:    "- old content\n+ new beautiful content",
				Added:   1,
				Removed: 1,
			},
		}
		time.Sleep(1 * time.Second)
		events <- domain.ToolEndEvent{CallID: "diff-1"}

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
