package main

import (
	"fmt"
	"math/rand"
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
			chunkSize := rand.Intn(5) + 1
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
		time.Sleep(500 * time.Millisecond)

		// 2. The Sticky List Test
		simulateTyping("# Stress Test: Sticky Lists\n\n")
		simulateTyping("I am about to write a list correctly. Each item SHOULD appear as soon as I finish it.\n\n")
		time.Sleep(1000 * time.Millisecond)

		simulateTyping("1. First Item (Should appear immediately)\n")
		time.Sleep(2000 * time.Millisecond) // Long pause to observe if it's visible

		simulateTyping("2. Second Item (Should also appear immediately)\n")
		time.Sleep(2000 * time.Millisecond)

		simulateTyping("3. Third Item (Wait for it...)\n")
		time.Sleep(2000 * time.Millisecond)

		simulateTyping("\nEnd of list. Did you see them appear one by one?\n\n")
		time.Sleep(1000 * time.Millisecond)

		// 3. The Sticky Blockquote Test
		simulateTyping("# Stress Test: Sticky Blockquotes\n\n")
		simulateTyping("Now testing blockquotes. Each line should appear as I write it.\n\n")
		time.Sleep(1000 * time.Millisecond)

		simulateTyping("> This is the first line of a quote.\n")
		time.Sleep(2000 * time.Millisecond) // Pause

		simulateTyping("> This is the second line.\n")
		time.Sleep(2000 * time.Millisecond)

		simulateTyping("> Finally the third line.\n")
		time.Sleep(2000 * time.Millisecond)

		simulateTyping("\nEnd of quote.\n\n")

		time.Sleep(1000 * time.Millisecond)
		events <- domain.DoneEvent{}
	}()

	renderer.Wait()
}
