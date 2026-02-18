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
	// Initialize UI with default config
	cfg := config.DefaultConfig()
	// Create UI Renderer
	renderer, err := compose.NewRenderer(os.Stdout, os.Stdin, cfg)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating renderer: %v\n", err)
		os.Exit(1)
	}

	// --- Feed Events Loop ---
	go func() {
		// Initial pause for UI setup
		time.Sleep(500 * time.Millisecond)

		// 1. Paragraphs
		renderer.Send(domain.TextEvent{Text: "# h1\n\n"})
		renderer.Send(domain.TextEvent{Text: "## h2\n\n"})
		renderer.Send(domain.TextEvent{Text: "### h3\n\n"})
		renderer.Send(domain.TextEvent{Text: "### Phase 1: Paragraphs\n\n"})
		renderer.Send(domain.TextEvent{Text: "First paragraph streaming...\n\n"})

		// 2. Lists
		renderer.Send(domain.TextEvent{Text: "### Phase 2: Lists Verification\n\n"})

		renderer.Send(domain.TextEvent{Text: "**Unordered List:**\n"})
		ulItems := []string{
			"* First bullet\n",
			"* Second bullet\n",
			"* Third bullet\n\n",
		}
		for _, item := range ulItems {
			renderer.Send(domain.TextEvent{Text: item})
			time.Sleep(300 * time.Millisecond)
		}

		renderer.Send(domain.TextEvent{Text: "**Ordered List:**\n"})
		items := []string{
			"1. First item\n",
			"1. Second item\n",
			"1. Third item\n\n",
		}
		for _, item := range items {
			renderer.Send(domain.TextEvent{Text: item})
			time.Sleep(300 * time.Millisecond)
		}

		renderer.Send(domain.TextEvent{Text: "**Nested List:**\n"})
		nestedItems := []string{
			"1. Parent Item\n",
			"   * Child A\n",
			"   * Child B\n",
			"1. Parent Item Two\n\n",
		}
		for _, item := range nestedItems {
			renderer.Send(domain.TextEvent{Text: item})
			time.Sleep(300 * time.Millisecond)
		}

		renderer.Send(domain.TextEvent{Text: "**Checklist:**\n"})
		checkItems := []string{
			"- [x] Done Task\n",
			"- [ ] Todo Task\n",
			"- [ ] Pending Task\n\n",
		}
		for _, item := range checkItems {
			renderer.Send(domain.TextEvent{Text: item})
			time.Sleep(300 * time.Millisecond)
		}

		// 3. Blockquotes
		renderer.Send(domain.TextEvent{Text: "### Phase 3: Blockquotes\n\n"})
		renderer.Send(domain.TextEvent{Text: "> This is a blockquote.\n\n> It spans multiple lines.\n\n"})

		// 4. Code Blocks
		renderer.Send(domain.TextEvent{Text: "### Phase 4: Code Blocks\n\n"})
		renderer.Send(domain.TextEvent{Text: "```go\nfunc main() {\n    fmt.Println(\"Hello\")\n}\n```\n\n"})

		// 5. Tool Spinner
		renderer.Send(domain.TextEvent{Text: "### Phase 5: Tool\n\n"})
		renderer.Send(domain.ToolStartEvent{CallID: "t1", ToolName: "test", Display: domain.StringDisplay("Working...")})
		time.Sleep(1 * time.Second)
		renderer.Send(domain.ToolEndEvent{CallID: "t1"})

		renderer.Send(domain.DoneEvent{})
	}()

	if err := renderer.Wait(); err != nil {
		os.Exit(1)
	}
}
