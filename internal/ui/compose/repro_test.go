package compose

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/charmbracelet/bubbles/spinner"
)

// TestDemo_StreamingStability replicates the streaming pattern of cmd/demo/main.go
// but with deterministic pauses and rigorous per-frame validation of the Status Bar position.
func TestDemo_StreamingStability(t *testing.T) {
	// Initialize harness: 80 wide, 24 high, cursor at 20 (SpaceBelow = 4)
	h := newHarnessFrameHarness(t, 80, 24, 20)

	lastCheckedFrame := 0
	lastVisualBottom := -1
	maxVisualBottomReached := -1
	inJump := false

	assertFrames := func() {
		frames := h.ViewFrames()
		for i := lastCheckedFrame; i < len(frames); i++ {
			f := frames[i]

			// viewHeight is the number of lines Bubble Tea is redrawing.
			viewHeight := strings.Count(f.View, "\n")
			// visualBottom is the physical line in the terminal where the view ends.
			// It includes TFL (permanent history), PrintlnLines (transient history
			// during flush), and viewHeight (dynamic area).
			visualBottom := f.TotalFlushedLines + f.PrintlnLines + viewHeight

			if lastVisualBottom != -1 && visualBottom < lastVisualBottom {
				if !inJump {
					t.Errorf("Frame %d: JUMP UP! Visual bottom dropped %d -> %d (TFL=%d, viewH=%d, maxAbs=%d, isPrinting=%v)",
						i, lastVisualBottom, visualBottom, f.TotalFlushedLines, viewHeight, f.MaxAbsoluteHeight, f.IsPrinting)
					inJump = true
				}
			} else if inJump && visualBottom >= maxVisualBottomReached {
				t.Logf("Frame %d: RECOVER! Visual bottom at %d (isPrinting=%v)", i, visualBottom, f.IsPrinting)
				inJump = false
			}

			if visualBottom > maxVisualBottomReached {
				maxVisualBottomReached = visualBottom
			}
			lastVisualBottom = visualBottom
		}
		lastCheckedFrame = len(frames)
	}

	// apply sends an event and processes the resulting side-effect loop,
	// inserting artificial re-renders (ticks) to verify stability during wait states.
	apply := func(ev domain.Event) {
		cmd := h.ApplyEventOnly(ev)
		assertFrames()

		iters := 0
		for cmd != nil && iters < 100 {
			// Simulate an animation Tick occurring while the engine is waiting for a command to finish
			// (e.g. while IsPrinting=true or while a tool is running).
			h.adapter.Update(spinner.TickMsg{})
			h.capture()
			assertFrames()

			// Advance the state machine
			cmd = h.ProcessCmd(cmd)
			assertFrames()
			iters++
		}

		// Final settled tick
		h.adapter.Update(spinner.TickMsg{})
		h.capture()
		assertFrames()
	}

	simulateTyping := func(text string) {
		runes := []rune(text)
		for i := 0; i < len(runes); {
			chunkSize := 4 // Deterministic chunks
			if i+chunkSize > len(runes) {
				chunkSize = len(runes) - i
			}
			apply(domain.TextEvent{Text: string(runes[i : i+chunkSize])})
			i += chunkSize
		}
	}

	// 1. Thinking
	apply(domain.ThinkingEvent{})

	// 2. Intro with Markdown features
	simulateTyping("# Integrated Architecture Validation\n\n")
	simulateTyping("I will now demonstrate the **new inline UI** capabilities. ")
	simulateTyping("This system uses a _streaming markdown parser_ to flush content block-by-block.\n\n")

	simulateTyping("Here is the plan:\n")
	simulateTyping("1. Test fragmentation handling\n")
	simulateTyping("2. Test code block streaming\n")
	simulateTyping("3. Test concurrent tool execution\n")
	simulateTyping("4. Test overflow handling for long blocks\n\n")

	// 3. Fragmentation Test
	simulateTyping("### Fragmentation Test\n\n")
	simulateTyping("This sentence has **bro")
	simulateTyping("ken bold** markers and `split")
	simulateTyping(" code` formatting. ")
	simulateTyping("this is ```inline ")
	simulateTyping("code block```\n\n")

	// 4. Code Block Streaming
	simulateTyping("Now writing a Go function:\n\n")
	apply(domain.TextEvent{Text: "```go\n"})

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
	}
	apply(domain.TextEvent{Text: "```\n\n"})
	simulateTyping("That block should now be flushed to history.\n\n")

	// 5. Concurrent Tools & Ordered Flushing
	simulateTyping("### Tool Execution & Ordering\n\n")
	simulateTyping("I'll start two tools. Tool B finishes FIRST, but should wait for Tool A.\n\n")

	// Start A
	apply(domain.ToolStartEvent{
		CallID:   "tool-A",
		ToolName: "long-job",
		Display:  domain.StringDisplay("Tool A: Long Running Job..."),
	})

	// Start B
	apply(domain.ToolStartEvent{
		CallID:   "tool-B",
		ToolName: "fast-job",
		Display:  domain.StringDisplay("Tool B: Quick Job"),
	})

	// Finish B (Success) - Should NOT flush yet
	apply(domain.ToolEndEvent{CallID: "tool-B"})

	// Finish A (Success) - Should flush A then B
	apply(domain.ToolEndEvent{CallID: "tool-A"})

	// 6. Long Block Overflow Test
	simulateTyping("\n### Overflow Indicator Test\n\n")
	simulateTyping("Generating a VERY long block to test standard output clipping and the overflow indicator:\n\n")

	for i := 0; i < 30; i++ {
		line := fmt.Sprintf("Line %03d: This is a generated line to fill the screen and force the pending block to overflow the viewport.\n", i+1)
		simulateTyping(line)
	}
	simulateTyping("\nEnd of long block.\n\n")

	// 7. Final Tool with Output
	apply(domain.ToolStartEvent{
		CallID:   "final-shell",
		ToolName: "shell",
		Display: domain.ShellDisplay{
			Header:  "Final Cleanup",
			Command: "rm -rf /tmp/demo",
		},
	})

	chunks := []string{"cleaning...", " removing files...", " done.\n"}
	for _, c := range chunks {
		apply(domain.ToolStreamEvent{CallID: "final-shell", Chunk: c})
	}
	apply(domain.ToolEndEvent{CallID: "final-shell"})

	// 8. Session End
	apply(domain.DoneEvent{})
}
