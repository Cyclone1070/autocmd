package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui/compose"
)

// --- CONFIGURATION ---
const (
	NumBlocks     = 8
	LinesPerBlock = 10 // Change this to test different jumping/flashing magnitudes
)

func main() {
	cfg := config.DefaultConfig()
	renderer, err := compose.NewRenderer(os.Stdout, os.Stdin, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	events := make(chan domain.Event, 100)
	go func() {
		for ev := range events {
			renderer.Send(ev)
		}
	}()

	simulateTyping := func(text string) {
		runes := []rune(text)
		for i := 0; i < len(runes); {
			chunkSize := 4
			if i+chunkSize > len(runes) {
				chunkSize = len(runes) - i
			}
			events <- domain.TextEvent{Text: string(runes[i : i+chunkSize])}
			i += chunkSize
			time.Sleep(10 * time.Millisecond) // Slow enough to see the view grow
		}
	}

	go func() {
		defer close(events)
		time.Sleep(500 * time.Millisecond)

		for i := 1; i <= NumBlocks; i++ {
			// Construct a block with exactly LinesPerBlock lines
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("### Block %d\n", i))
			for j := 1; j < LinesPerBlock; j++ {
				sb.WriteString(fmt.Sprintf("Line %d of block %d content...\n", j, i))
			}
			sb.WriteString("\n") // Double newline to trigger markdown flush

			simulateTyping(sb.String())

			// Optional thinking/pause between blocks to stabilize
			events <- domain.ThinkingEvent{}
			time.Sleep(500 * time.Millisecond)
		}

		time.Sleep(500 * time.Millisecond)
		events <- domain.DoneEvent{}
	}()

	renderer.Wait()
}
