package main

import (
	"fmt"
	"os"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
)

func main() {
	cfg := config.DefaultConfig()
	renderer, err := ui.NewRenderer(os.Stdout, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating renderer: %v\n", err)
		os.Exit(1)
	}
	events := make(chan domain.Event, 10)

	go func() {
		for ev := range events {
			renderer.Send(ev)
		}
	}()

	go func() {
		defer close(events)
		time.Sleep(300 * time.Millisecond)

		// 1. Thinking
		events <- domain.ThinkingEvent{}
		time.Sleep(800 * time.Millisecond)

		// 2. Text (Context) with fragmented markdown
		events <- domain.TextEvent{Text: "Running comprehensive UI consistency check...\n\n"}
		time.Sleep(300 * time.Millisecond)

		mdChunks := []string{
			"Checking **bold",
			" text** rendering...\n",
			"Also _italic",
			" text_ support.\n\n",
		}
		for _, c := range mdChunks {
			events <- domain.TextEvent{Text: c}
			time.Sleep(200 * time.Millisecond)
		}

		// SCENARIO 1: String Display (Success vs Failure)
		// Concurrent execution
		events <- domain.ToolStartEvent{
			CallID:   "s_success",
			ToolName: "read_file",
			Display:  domain.StringDisplay("Reading config.yaml"),
		}
		time.Sleep(100 * time.Millisecond)

		events <- domain.ToolStartEvent{
			CallID:   "s_fail",
			ToolName: "read_file",
			Display:  domain.StringDisplay("Reading secret.key"),
		}

		time.Sleep(600 * time.Millisecond)
		events <- domain.ToolEndEvent{CallID: "s_success"}
		time.Sleep(200 * time.Millisecond)
		events <- domain.ToolEndEvent{CallID: "s_fail", Error: "permission denied"}
		time.Sleep(500 * time.Millisecond)

		// SCENARIO 2: Diff Display (Success vs Failure)
		// Concurrent
		events <- domain.ToolStartEvent{
			CallID:   "d_success",
			ToolName: "edit_file",
			Display: domain.DiffDisplay{
				Header:  "Edit main.go",
				Added:   5,
				Removed: 1,
				Diff:    "-oldVar := 1\n+newVar := 2\n+addedVar := 3",
			},
		}
		time.Sleep(100 * time.Millisecond)

		events <- domain.ToolStartEvent{
			CallID:   "d_fail",
			ToolName: "edit_file",
			Display: domain.DiffDisplay{
				Header:  "Edit locked.go",
				Added:   2,
				Removed: 0,
				Diff:    "+lockedFunc() {}",
			},
		}

		time.Sleep(600 * time.Millisecond)
		events <- domain.ToolEndEvent{CallID: "d_success"}
		time.Sleep(200 * time.Millisecond)
		events <- domain.ToolEndEvent{CallID: "d_fail", Error: "file is locked by another process"}
		time.Sleep(500 * time.Millisecond)

		// SCENARIO 3: Shell Display (Success vs Failure)
		// Concurrent
		events <- domain.ToolStartEvent{
			CallID:   "sh_success",
			ToolName: "shell",
			Display: domain.ShellDisplay{
				Header:  "Build Project",
				Command: "go build ./...",
			},
		}
		time.Sleep(100 * time.Millisecond)

		events <- domain.ToolStartEvent{
			CallID:   "sh_fail",
			ToolName: "shell",
			Display: domain.ShellDisplay{
				Header:  "Run Tests",
				Command: "go test ./...",
			},
		}

		// Stream content to both
		chunks := []string{".", "..", "..."}
		for _, c := range chunks {
			events <- domain.ToolStreamEvent{CallID: "sh_success", Chunk: c}
			events <- domain.ToolStreamEvent{CallID: "sh_fail", Chunk: c}
			time.Sleep(200 * time.Millisecond)
		}

		events <- domain.ToolEndEvent{CallID: "sh_success"}
		time.Sleep(200 * time.Millisecond)
		events <- domain.ToolEndEvent{CallID: "sh_fail", Error: "compilation failed"}
		time.Sleep(500 * time.Millisecond)

		events <- domain.DoneEvent{}
	}()

	renderer.Wait()
}
