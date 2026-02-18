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

		// 1. The Sticky List Test
		events <- domain.TextEvent{Text: "# Stress Test: Sticky Lists\n\n"}
		events <- domain.TextEvent{Text: "I am about to write a list correctly. Each item SHOULD appear as soon as I finish it.\n\n"}
		time.Sleep(1000 * time.Millisecond)

		events <- domain.TextEvent{Text: "1. First Item (Should appear immediately)\n"}
		time.Sleep(2000 * time.Millisecond) // Long pause to observe if it's visible

		events <- domain.TextEvent{Text: "2. Second Item (Should also appear immediately)\n"}
		time.Sleep(2000 * time.Millisecond)

		events <- domain.TextEvent{Text: "3. Third Item (Wait for it...)\n"}
		time.Sleep(2000 * time.Millisecond)

		events <- domain.TextEvent{Text: "\nEnd of list. Did you see them appear one by one?\n\n"}
		time.Sleep(1000 * time.Millisecond)

		// 3. The Sticky Blockquote Test
		events <- domain.TextEvent{Text: "# Stress Test: Sticky Blockquotes\n\n"}
		events <- domain.TextEvent{Text: "Now testing blockquotes. Each line should appear as I write it.\n\n"}
		time.Sleep(1000 * time.Millisecond)

		events <- domain.TextEvent{Text: "> This is the first line of a quote.\n"}
		time.Sleep(2000 * time.Millisecond) // Pause

		events <- domain.TextEvent{Text: "> This is the second line.\n"}
		time.Sleep(2000 * time.Millisecond)

		events <- domain.TextEvent{Text: "> Finally the third line.\n"}
		time.Sleep(2000 * time.Millisecond)

		events <- domain.TextEvent{Text: "\nEnd of quote.\n\n"}

		time.Sleep(1000 * time.Millisecond)
		events <- domain.DoneEvent{}
	}()

	renderer.Wait()
}
