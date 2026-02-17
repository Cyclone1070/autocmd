package main

import (
	"fmt"
	"os"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui/compose"
)

func main() {
	cfg := config.DefaultConfig()
	// Create UI Renderer
	renderer, err := compose.NewRenderer(os.Stdout, os.Stdin, cfg)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating renderer: %v\n", err)
		os.Exit(1)
	}
	events := make(chan domain.Event, 100)

	// Helper for natural typing simulation
	simulateTyping := func(text string) {
		// Split into small chunks of 1-5 chars
		runes := []rune(text)
		cursor := 0
		for cursor < len(runes) {
			chunkSize := 4
			if cursor+chunkSize > len(runes) {
				chunkSize = len(runes) - cursor
			}

			chunk := string(runes[cursor : cursor+chunkSize])
			events <- domain.TextEvent{Text: chunk}
			cursor += chunkSize

			// Variable delay 10-50ms
			time.Sleep(time.Duration(10 * time.Millisecond))
		}
	}

	go func() {
		for ev := range events {
			renderer.Send(ev)
		}
	}()

	go func() {
		defer close(events)
		time.Sleep(500 * time.Millisecond)

		// 1. Thinking
		events <- domain.ThinkingEvent{}
		time.Sleep(1000 * time.Millisecond)

		// 2. Intro with Markdown features (Paragraphs, Bold, List)
		simulateTyping("# Integrated Architecture Validation\n\n")
		time.Sleep(300 * time.Millisecond)

		simulateTyping("I will now demonstrate the **new inline UI** capabilities. ")
		simulateTyping("This system uses a _streaming markdown parser_ to flush content block-by-block.\n\n")

		simulateTyping("Here is the plan:\n")
		simulateTyping("1. Test fragmentation handling\n")
		simulateTyping("2. Test code block streaming\n")
		simulateTyping("3. Test concurrent tool execution\n")
		simulateTyping("4. Test overflow handling for long blocks\n\n")

		time.Sleep(500 * time.Millisecond)

		// 3. Fragmentation Test (Partial markdown markers)
		simulateTyping("### Fragmentation Test\n\n")
		simulateTyping("This sentence has **bro")
		time.Sleep(500 * time.Millisecond) // Pause mid-bold
		simulateTyping("ken bold** markers and `split")
		time.Sleep(500 * time.Millisecond) // Pause mid-code
		simulateTyping(" code` formatting. ")
		time.Sleep(500 * time.Millisecond)
		simulateTyping("this is ```inline ")
		time.Sleep(500 * time.Millisecond)
		simulateTyping("code block```\n\n")

		// 4. Code Block Streaming (Unsafe -> Safe transition)
		simulateTyping("Now writing a Go function:\n\n")

		events <- domain.TextEvent{Text: "```go\n"} // Start block
		time.Sleep(500 * time.Millisecond)

		codeLines := []string{
			"package main\n\n",
			"func hello() {\n",
			"    fmt.Println(\"Hello World\")\n",
			"    // Simulating complex logic...\n",
			"    time.Sleep(1 * time.Second)\n",
			"}\n",
		}

		for _, line := range codeLines {
			simulateTyping(line)
			time.Sleep(200 * time.Millisecond)
		}

		events <- domain.TextEvent{Text: "```\n\n"} // Close block (should trigger flush)
		simulateTyping("That block should now be flushed to history.\n\n")
		time.Sleep(1000 * time.Millisecond)

		// 5. Concurrent Tools & Ordered Flushing
		simulateTyping("### Tool Execution & Ordering\n\n")
		simulateTyping("I'll start two tools. Tool B finishes FIRST, but should wait for Tool A.\n\n")

		// Start A
		events <- domain.ToolStartEvent{
			CallID:   "tool-A",
			ToolName: "long-job",
			Display:  domain.StringDisplay("Tool A: Long Running Job..."),
		}
		time.Sleep(200 * time.Millisecond)

		// Start B
		events <- domain.ToolStartEvent{
			CallID:   "tool-B",
			ToolName: "fast-job",
			Display:  domain.StringDisplay("Tool B: Quick Job"),
		}
		time.Sleep(500 * time.Millisecond)

		// Finish B (Success) - Should NOT flush yet
		events <- domain.ToolEndEvent{CallID: "tool-B"}
		time.Sleep(1500 * time.Millisecond) // Long pause to verify B waits

		// Finish A (Success) - Should flush A then B
		events <- domain.ToolEndEvent{CallID: "tool-A"}
		time.Sleep(500 * time.Millisecond)

		// 6. Long Block Overflow Test
		simulateTyping("\n### Overflow Indicator Test\n\n")
		simulateTyping("Generating a VERY long block to test standard output clipping and the overflow indicator:\n\n")

		// Generate 30 lines of text (enough to overflow typical terminals)
		for i := range 30 {
			line := fmt.Sprintf("Line %03d: This is a generated line to fill the screen and force the pending block to overflow the viewport.\n", i+1)
			simulateTyping(line)
		}
		simulateTyping("\nEnd of long block.\n\n")
		time.Sleep(500 * time.Millisecond) // Reduced read pause

		// 7. Final Tool with Output
		events <- domain.ToolStartEvent{
			CallID:   "final-shell",
			ToolName: "shell",
			Display: domain.ShellDisplay{
				Header:  "Final Cleanup",
				Command: "rm -rf /tmp/demo",
			},
		}

		chunks := []string{"cleaning...", " removing files...", " done.\n"}
		for _, c := range chunks {
			events <- domain.ToolStreamEvent{CallID: "final-shell", Chunk: c}
			time.Sleep(300 * time.Millisecond)
		}
		events <- domain.ToolEndEvent{CallID: "final-shell"}

		time.Sleep(500 * time.Millisecond)
		events <- domain.DoneEvent{}
	}()

	renderer.Wait()
}
