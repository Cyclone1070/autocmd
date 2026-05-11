package main

import (
	"fmt"
	"os"
	"time"

	"github.com/Cyclone1070/iav/internal/eventbus"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/Cyclone1070/iav/internal/ui/prompt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	chatWidth      = 80
	thinkingHeight = 5
	demoDelay      = 500 * time.Millisecond
	thinkingDelay  = 2 * time.Second
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("Fatal error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	bus := eventbus.New()
	theme := ui.NewTheme(ui.ThemeConfig{
		PrimaryColor: lipgloss.AdaptiveColor{Light: "#0EA5E9", Dark: "#38BDF8"},
		SuccessColor: lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"},
		ErrorColor:   lipgloss.AdaptiveColor{Light: "#F05D5E", Dark: "#FF6666"},
		MutedColor:   lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#888888"},
	})

	glamour := ui.NewGlamourRenderer(chatWidth, true)
	stream := prompt.NewStream(glamour)
	spinner := ui.NewSpinnerRenderer(lipgloss.NewStyle().Foreground(theme.PrimaryColor()))
	thinking := prompt.NewThinkingRenderer(theme, chatWidth, ui.NewToolOutputGater(thinkingHeight))
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
		time.Sleep(demoDelay)
		time.Sleep(thinkingDelay)
		bus.Close()
	}()

	p := tea.NewProgram(uiModel)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running program: %w", err)
	}
	return nil
}
