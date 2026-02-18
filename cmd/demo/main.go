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

	go func() {
		for ev := range events {
			renderer.Send(ev)
		}
	}()

	go func() {
		defer close(events)
		time.Sleep(500 * time.Millisecond)

		// 2. Intro with Markdown features (Paragraphs, Bold, List)
		events <- domain.TextEvent{Text: "# Integrated Architecture Validation\n\n"}
		time.Sleep(300 * time.Millisecond)

		events <- domain.TextEvent{Text: "I will now demonstrate the **new inline UI** capabilities. "}
		events <- domain.TextEvent{Text: "This system uses a _streaming markdown parser_ to flush content block-by-block.\n\n"}

		events <- domain.TextEvent{Text: "Here is the plan:\n"}
		events <- domain.TextEvent{Text: "1. Test fragmentation handling\n"}
		events <- domain.TextEvent{Text: "2. Test code block streaming\n"}
		events <- domain.TextEvent{Text: "3. Test concurrent tool execution\n"}
		events <- domain.TextEvent{Text: "4. Test overflow handling for long blocks\n\n"}

		time.Sleep(500 * time.Millisecond)

		// 3. Fragmentation Test (Partial markdown markers)
		events <- domain.TextEvent{Text: "### Fragmentation Test\n\n"}
		events <- domain.TextEvent{Text: "This sentence has **bro"}
		time.Sleep(500 * time.Millisecond) // Pause mid-bold
		events <- domain.TextEvent{Text: "ken bold** markers and `split"}
		time.Sleep(500 * time.Millisecond) // Pause mid-code
		events <- domain.TextEvent{Text: " code` formatting. "}
		time.Sleep(500 * time.Millisecond)
		events <- domain.TextEvent{Text: "this is ```inline "}
		time.Sleep(500 * time.Millisecond)
		events <- domain.TextEvent{Text: "code block```\n\n"}

		// 4. Code Block Streaming (Unsafe -> Safe transition)
		events <- domain.TextEvent{Text: "Now writing a Go function:\n\n"}

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
			events <- domain.TextEvent{Text: line}
			time.Sleep(200 * time.Millisecond)
		}

		events <- domain.TextEvent{Text: "```\n\n"} // Close block (should trigger flush)
		events <- domain.TextEvent{Text: "That block should now be flushed to history.\n\n"}
		time.Sleep(1000 * time.Millisecond)

		// 5. Concurrent Tools & Ordered Flushing
		events <- domain.TextEvent{Text: "### Tool Execution & Ordering\n\n"}
		events <- domain.TextEvent{Text: "I'll start two tools. Tool B finishes FIRST, but should wait for Tool A.\n\n"}

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
		events <- domain.TextEvent{Text: "\n### Overflow Indicator Test\n\n"}
		events <- domain.TextEvent{Text: "Generating a VERY long block to test standard output clipping and the overflow indicator:\n\n"}

		// Generate 30 lines of text (enough to overflow typical terminals)
		for i := 0; i < 30; i++ {
			line := fmt.Sprintf("Line %03d: This is a generated line to fill the screen and force the pending block to overflow the viewport.\n", i+1)
			events <- domain.TextEvent{Text: line}
		}
		events <- domain.TextEvent{Text: "\nEnd of long block.\n\n"}
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
