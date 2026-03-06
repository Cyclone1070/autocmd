package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui/loop"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	events := make(chan domain.Event)
	uiCfg := config.DefaultConfig().UI
	uiCfg.ChatWindowWidth = 80
	m := loop.NewModel(events, uiCfg)

	p := tea.NewProgram(m)

	// Long strings for overflow testing
	longString := strings.Repeat("OverflowingContent_", 10)
	longHeader := "This is a very long header that will definitely exceed the eighty character limit of the tool box"
	longCommand := "sh -c 'echo \"This is a very long command line that will also definitely exceed the eighty character limit of the tool box\" && sleep 1'"
	longOutput := strings.Repeat("LongOutputLineContent_", 10) + "\n" + strings.Repeat("AnotherLongLine-", 15)

	go func() {
		// 1. StringDisplay Overflow
		events <- domain.ToolStartEvent{
			CallID:  "string-overflow",
			Display: domain.NewStringDisplay("Short Header"),
		}
		time.Sleep(500 * time.Millisecond)
		events <- domain.ToolEndEvent{
			CallID: "string-overflow",
		}
		time.Sleep(500 * time.Millisecond)

		events <- domain.ToolStartEvent{
			CallID:  "string-overflow-2",
			Display: domain.NewStringDisplay(longHeader),
		}
		time.Sleep(500 * time.Millisecond)
		events <- domain.ToolEndEvent{
			CallID: "string-overflow-2",
		}
		time.Sleep(500 * time.Millisecond)

		// 2. DiffDisplay Overflow
		events <- domain.ToolStartEvent{
			CallID:  "diff-overflow",
			Display: domain.NewDiffDisplay(longHeader, "Edit "+longString, 1, 1, "+ "+longString+"\n- "+longString),
		}
		time.Sleep(500 * time.Millisecond)
		events <- domain.ToolEndEvent{CallID: "diff-overflow"}
		time.Sleep(500 * time.Millisecond)

		// 3. ShellDisplay Overflow (All parts)
		events <- domain.ToolStartEvent{
			CallID:  "shell-overflow",
			Display: domain.NewShellDisplay(longHeader, longCommand, nil, nil),
		}
		time.Sleep(500 * time.Millisecond)
		events <- domain.ToolStreamEvent{
			CallID: "shell-overflow",
			Chunk:  longOutput,
		}
		time.Sleep(500 * time.Millisecond)
		events <- domain.ToolEndEvent{CallID: "shell-overflow"}
		time.Sleep(500 * time.Millisecond)

		// Done
		events <- domain.DoneEvent{}
	}()

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
