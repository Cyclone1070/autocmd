package cmd

import (
	"fmt"
	"os"

	"github.com/Cyclone1070/iav/internal/eventbus"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/Cyclone1070/iav/internal/ui/history"
	"github.com/Cyclone1070/iav/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func init() {
	rootCmd.AddCommand(historyCmd)
}

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "View chat history for the current session",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := Wire()
		if err != nil {
			return err
		}

		bus := eventbus.New()
		defer bus.Close()

		done := workflow.RunHistory(cmd.Context(), &workflow.HistoryDeps{
			Store: deps.SessionStore,
			State: deps.State,
		}, bus)

		themeCfg := ui.ThemeConfig{
			PrimaryColor: ui.ToAdaptiveColor(deps.Config.UI().PrimaryColor()),
			SuccessColor: ui.ToAdaptiveColor(deps.Config.UI().SuccessColor()),
			ErrorColor:   ui.ToAdaptiveColor(deps.Config.UI().ErrorColor()),
			MutedColor:   ui.ToAdaptiveColor(deps.Config.UI().MutedColor()),
			ShortToolbox: deps.Config.UI().ShortToolbox(),
		}
		theme := ui.NewTheme(themeCfg)

		width, height, _ := term.GetSize(int(os.Stdout.Fd()))
		m := history.NewModel(bus, theme, deps.Config.UI().ChatWindowWidth(), width, height)
		p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

		if _, err := p.Run(); err != nil {
			return fmt.Errorf("failed to run history viewer: %w", err)
		}

		if err := <-done; err != nil {
			return err
		}

		return nil
	},
}
