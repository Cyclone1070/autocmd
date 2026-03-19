package cmd

import (
	"fmt"
	"os"

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

		res, err := workflow.ResolveSession(&workflow.HistoryDeps{
			Store: deps.SessionStore,
			State: deps.State,
		})
		if err != nil {
			return err
		}
		sess := res.Session

		themeCfg := ui.ThemeConfig{
			PrimaryColor: ui.ToAdaptiveColor(deps.Config.UI().PrimaryColor()),
			SuccessColor: ui.ToAdaptiveColor(deps.Config.UI().SuccessColor()),
			ErrorColor:   ui.ToAdaptiveColor(deps.Config.UI().ErrorColor()),
			MutedColor:   ui.ToAdaptiveColor(deps.Config.UI().MutedColor()),
			ShortToolbox: deps.Config.UI().ShortToolbox(),
		}

		width, height, _ := term.GetSize(int(os.Stdout.Fd()))
		m := history.NewModel(sess.Messages, sess.ToolDisplays, themeCfg, deps.Config.UI().ChatWindowWidth(), width, height)
		// Enable mouse input so the viewport can scroll with the wheel/trackpad.
		p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

		if _, err := p.Run(); err != nil {
			return fmt.Errorf("failed to run history viewer: %w", err)
		}

		return nil
	},
}
