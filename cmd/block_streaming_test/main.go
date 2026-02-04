package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
)

func main() {
	// Initialize UI with default config
	cfg := config.DefaultConfig()
	// Create UI Renderer
	cursorDetector := ui.NewTerminalCursorDetector(os.Stdin, os.Stdout)
	renderer, err := ui.NewRenderer(os.Stdout, cfg, cursorDetector)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating renderer: %v\n", err)
		os.Exit(1)
	}

	// Channel to send events from our simulation to the UI
	events := make(chan domain.Event, 100)

	// --- Helper: Simulate Typing ---
	simulateTyping := func(text string) {
		runes := []rune(text)
		cursor := 0
		for cursor < len(runes) {
			chunkSize := rand.Intn(5) + 1
			if cursor+chunkSize > len(runes) {
				chunkSize = len(runes) - cursor
			}

			chunk := string(runes[cursor : cursor+chunkSize])

			// Send chunk to UI
			renderer.Send(domain.TextEvent{Text: chunk})

			cursor += chunkSize

			// Visual delay
			time.Sleep(20 * time.Millisecond)
		}
	}

	// --- Feed Events Loop ---
	go func() {
		defer close(events)

		// Initial pause for UI setup
		time.Sleep(500 * time.Millisecond)

		// 1. Paragraphs
		renderer.Send(domain.TextEvent{Text: "# h1\n\n"})
		renderer.Send(domain.TextEvent{Text: "## h2\n\n"})
		renderer.Send(domain.TextEvent{Text: "### h3\n\n"})
		simulateTyping("### Phase 1: Paragraphs\n\n")
		simulateTyping("First paragraph streaming...\n\n")

		// 2. Lists
		simulateTyping("### Phase 2: Lists Verification\n\n")

		simulateTyping("**Unordered List:**\n")
		ulItems := []string{
			"* First bullet\n",
			"* Second bullet\n",
			"* Third bullet\n\n",
		}
		for _, item := range ulItems {
			simulateTyping(item)
			time.Sleep(300 * time.Millisecond)
		}

		simulateTyping("**Ordered List:**\n")
		items := []string{
			"1. First item\n",
			"1. Second item\n",
			"1. Third item\n\n",
		}
		for _, item := range items {
			simulateTyping(item)
			time.Sleep(300 * time.Millisecond)
		}

		simulateTyping("**Nested List:**\n")
		nestedItems := []string{
			"1. Parent Item\n",
			"   * Child A\n",
			"   * Child B\n",
			"1. Parent Item Two\n\n",
		}
		for _, item := range nestedItems {
			simulateTyping(item)
			time.Sleep(300 * time.Millisecond)
		}

		simulateTyping("**Checklist:**\n")
		checkItems := []string{
			"- [x] Done Task\n",
			"- [ ] Todo Task\n",
			"- [ ] Pending Task\n\n",
		}
		for _, item := range checkItems {
			simulateTyping(item)
			time.Sleep(300 * time.Millisecond)
		}

		// 3. Blockquotes
		simulateTyping("### Phase 3: Blockquotes\n\n")
		simulateTyping("> This is a blockquote.\n\n> It spans multiple lines.\n\n")

		// 4. Code Blocks
		simulateTyping("### Phase 4: Code Blocks\n\n")
		simulateTyping("```go\nfunc main() {\n    fmt.Println(\"Hello\")\n}\n```\n\n")

		// 5. Tool Spinner
		simulateTyping("### Phase 5: Tool\n\n")
		renderer.Send(domain.ToolStartEvent{CallID: "t1", ToolName: "test", Display: domain.StringDisplay("Working...")})
		time.Sleep(1 * time.Second)
		renderer.Send(domain.ToolEndEvent{CallID: "t1"})

		renderer.Send(domain.DoneEvent{})
	}()

	if err := renderer.Wait(); err != nil {
		os.Exit(1)
	}
}
