package main

import (
	"fmt"
	"os"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/Cyclone1070/iav/internal/ui/prompt"
	"github.com/Cyclone1070/iav/internal/eventbus"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func main() {
	bus := eventbus.New()
	theme := ui.NewTheme(ui.ThemeConfig{
		PrimaryColor: lipgloss.AdaptiveColor{Light: "#0EA5E9", Dark: "#38BDF8"},
		SuccessColor: lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"},
		ErrorColor:   lipgloss.AdaptiveColor{Light: "#F05D5E", Dark: "#FF6666"},
		MutedColor:   lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#888888"},
	})

	chatWidth := 80
	glamour := ui.NewGlamourRenderer(chatWidth, true)
	stream := prompt.NewStream(glamour)
	spinner := ui.NewSpinnerRenderer(lipgloss.NewStyle().Foreground(theme.PrimaryColor()))
	thinking := prompt.NewThinkingRenderer(theme, chatWidth, ui.NewToolOutputGater(5))
	tooling := ui.NewToolRenderer(theme, chatWidth, ui.NewNoOpGater())

	uiModel := prompt.NewModel(
		bus,
		thinking,
		tooling,
		spinner,
		theme,
		stream,
		ui.NewNoOpGater(),
		chatWidth,
	)

	go func() {
		time.Sleep(500 * time.Millisecond)
		longText := "This is a long piece of text that will be streamed and animated. It contains enough content to ensure the animator is still active when the bus is closed suddenly in the middle of our communication pipe."
		bus.SendUIUpdate(domain.TextEvent{Text: longText})
		time.Sleep(1 * time.Second)
		bus.Close()
	}()

	p := tea.NewProgram(uiModel)
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
