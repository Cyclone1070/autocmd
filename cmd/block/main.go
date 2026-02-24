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

	go func() {
		blocks := []string{
			"# H1 Header - Should be uppercase\n",
			"## H2 Header\n",
			"### H3 Header\n",
			"This is a paragraph with **bold** and *italic* and `inline code`.\n",
			"Here is a list:\n- Item 1\n- Item 2\n- Item 3\n",
			"1. Ordered item 1\n2. Ordered item 2\n",
			"> This is a blockquote.\n> It can have multiple lines.\n",
			"```go\nfunc hello() {\n    fmt.Println(\"Hello, World!\")\n}\n```\n",
			"---\n", // HR
			"| Table | Header |\n|-------|--------|\n| Row 1 | Cell 1 |\n| Row 2 | Cell 2 |\n\n",
			"Task list:\n- [x] Done\n- [ ] Todo\n",
		}

		for _, b := range blocks {
			events <- domain.TextEvent{Text: b}
			time.Sleep(300 * time.Millisecond) // Slow stream to watch it build
		}

		events <- domain.DoneEvent{}
	}()

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
