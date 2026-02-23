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

	// Simulate workflow in background
	go func() {
		// 1. Thinking
		events <- domain.ThinkingEvent{}

		events <- domain.TextEvent{Text: "1st text: This text should be above tool call at all stages of the display."}

		events <- domain.ToolStartEvent{
			CallID:  "tool-0",
			Display: domain.StringDisplay("This tool call display should be sandwiched."),
		}
		time.Sleep(1 * time.Second)
		events <- domain.ToolEndEvent{CallID: "tool-0"}

		events <- domain.TextEvent{Text: "2nd text: This text should be bellow tool call at all stages of the display."}
		events <- domain.DoneEvent{}
	}()

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
