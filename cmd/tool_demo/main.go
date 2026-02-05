package main

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
)

func main() {
	cfg := config.DefaultConfig()
	// Create UI Renderer
	cursorDetector := ui.NewTerminalCursorDetector(os.Stdin, os.Stdout)
	renderer, err := ui.NewRenderer(os.Stdout, cfg, cursorDetector)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating renderer: %v\n", err)
		os.Exit(1)
	}

	// Run all batches in a separate goroutine so they don't block the main thread
	go func() {
		// 1. Batch One: String Tools (Concurrent Execution, Deterministic Start)
		var wg1 sync.WaitGroup
		wg1.Add(3)

		// Start Events (Sequential - LOCKS ORDER)
		renderer.Send(domain.TextEvent{Text: "Starting String Tools...\n"})
		renderer.Send(domain.ToolStartEvent{CallID: "str-long", ToolName: "String: Long Success", Display: domain.StringDisplay("Processing large dataset...")})
		renderer.Send(domain.ToolStartEvent{CallID: "str-short", ToolName: "String: Short Success", Display: domain.StringDisplay("Quick check done.")})
		renderer.Send(domain.ToolStartEvent{CallID: "str-fail", ToolName: "String: Failure", Display: domain.StringDisplay("Validation failed.")})

		// Execution (Concurrent)
		go func() {
			defer wg1.Done()
			time.Sleep(5 * time.Second)
			renderer.Send(domain.ToolEndEvent{CallID: "str-long"})
		}()
		go func() {
			defer wg1.Done()
			time.Sleep(1 * time.Second)
			renderer.Send(domain.ToolEndEvent{CallID: "str-short"})
		}()
		go func() {
			defer wg1.Done()
			time.Sleep(2 * time.Second)
			renderer.Send(domain.ToolEndEvent{CallID: "str-fail", Error: "Access denied"})
		}()
		wg1.Wait()

		time.Sleep(500 * time.Millisecond)

		// 2. Batch Two: Diff Tools
		diffSuccess := domain.DiffDisplay{
			Header:  "Update config.yaml",
			Added:   5,
			Removed: 2,
			Diff:    "--- config.yaml\n+++ config.yaml\n@@ -1,3 +1,6 @@\n-old_value: true\n+new_value: false\n+added_line: 1\n+added_line: 2",
		}
		diffFail := domain.DiffDisplay{
			Header: "Apply Patch",
			Diff:   "Conflicting changes in main.go",
		}

		var wg2 sync.WaitGroup
		wg2.Add(3)

		// Start Events
		renderer.Send(domain.TextEvent{Text: "\nStarting Diff Tools...\n"})
		renderer.Send(domain.ToolStartEvent{CallID: "diff-long", ToolName: "Diff: Long Success", Display: diffSuccess})
		renderer.Send(domain.ToolStartEvent{CallID: "diff-short", ToolName: "Diff: Short Success", Display: diffSuccess})
		renderer.Send(domain.ToolStartEvent{CallID: "diff-fail", ToolName: "Diff: Failure", Display: diffFail})

		// Execution
		go func() {
			defer wg2.Done()
			time.Sleep(5 * time.Second)
			renderer.Send(domain.ToolEndEvent{CallID: "diff-long"})
		}()
		go func() {
			defer wg2.Done()
			time.Sleep(1 * time.Second)
			renderer.Send(domain.ToolEndEvent{CallID: "diff-short"})
		}()
		go func() {
			defer wg2.Done()
			time.Sleep(2 * time.Second)
			renderer.Send(domain.ToolEndEvent{CallID: "diff-fail", Error: "Merge conflict"})
		}()
		wg2.Wait()

		time.Sleep(500 * time.Millisecond)

		// 3. Batch Three: Shell Tools
		var wg3 sync.WaitGroup
		wg3.Add(3)

		// Start Events
		renderer.Send(domain.TextEvent{Text: "\nStarting Shell Tools...\n"})
		renderer.Send(domain.ToolStartEvent{CallID: "sh-long", ToolName: "Shell: Long Success", Display: domain.ShellDisplay{Header: "Shell: Long Success", Command: "npm install"}})
		renderer.Send(domain.ToolStartEvent{CallID: "sh-short", ToolName: "Shell: Short Success", Display: domain.ShellDisplay{Header: "Shell: Short Success", Command: "ls -la"}})
		renderer.Send(domain.ToolStartEvent{CallID: "sh-fail", ToolName: "Shell: Failure", Display: domain.ShellDisplay{Header: "Shell: Failure", Command: "make build"}})

		// Execution
		go func() {
			defer wg3.Done()
			streamShellOutput(renderer, "sh-long", 5)
			renderer.Send(domain.ToolEndEvent{CallID: "sh-long"})
		}()
		go func() {
			defer wg3.Done()
			streamShellOutput(renderer, "sh-short", 1)
			renderer.Send(domain.ToolEndEvent{CallID: "sh-short"})
		}()
		go func() {
			defer wg3.Done()
			streamShellOutput(renderer, "sh-fail", 2)
			renderer.Send(domain.ToolEndEvent{CallID: "sh-fail", Error: "Exit status 1"})
		}()
		wg3.Wait()

		time.Sleep(500 * time.Millisecond)
		renderer.Send(domain.DoneEvent{})
	}()

	if err := renderer.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func simulateTool(r *ui.Renderer, id, name string, display domain.ToolDisplay, duration time.Duration, errStr string) {
	r.Send(domain.ToolStartEvent{
		CallID:   id,
		ToolName: name,
		Display:  display,
	})
	time.Sleep(duration)
	r.Send(domain.ToolEndEvent{
		CallID: id,
		Error:  errStr,
	})
}

func streamShellOutput(r *ui.Renderer, id string, durationSec int) {
	for i := 0; i < durationSec*2; i++ {
		time.Sleep(500 * time.Millisecond)
		r.Send(domain.ToolStreamEvent{
			CallID: id,
			Chunk:  fmt.Sprintf("Step %d of %d in progress...\n", i+1, durationSec*2),
		})
	}
}
