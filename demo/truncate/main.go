package main

import (
	"fmt"
	"os"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui/loop"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	events := make(chan domain.Event)
	m := loop.NewModel(events, config.DefaultConfig().UI)

	p := tea.NewProgram(m)

	go func() {
		events <- domain.TextEvent{Text: "This is a test of the truncation feature. The following lines should be truncated if they exceed the terminal width.\n\n"}
		for i := 1; i <= 40; i++ {
			events <- domain.TextEvent{Text: fmt.Sprintf("Line %d: This is a repeated line for truncation testing it's gonna be quite long to overflow my goated terminal.\n", i)}
		}
		events <- domain.DoneEvent{}
	}()

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
