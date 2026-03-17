package cmd

import (
	"fmt"
	"os"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/Cyclone1070/iav/internal/state"
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
	Use:   "history [session_id]",
	Short: "View chat history for a session",
	RunE: func(cmd *cobra.Command, args []string) error {
		bootstrapFS := fs.NewOSFileSystem(-1)

		configMgr := config.NewManager(bootstrapFS)
		cfg, err := configMgr.Load()
		if err != nil {
			return err
		}

		stateMgr := state.NewManager(bootstrapFS)
		appState, err := stateMgr.Load()
		if err != nil {
			return err
		}

		store, err := buildSessionStore(cfg, bootstrapFS)
		if err != nil {
			return err
		}

		var argSessionID string
		if len(args) > 0 {
			argSessionID = args[0]
		}

		res, err := workflow.RunHistory(&workflow.HistoryDeps{
			Store: store,
			State: appState,
		}, argSessionID)
		if err != nil {
			return err
		}
		sess := res.Session

		themeCfg := ui.ThemeConfig{
			PrimaryColor: ui.ToAdaptiveColor(cfg.UI().PrimaryColor()),
			SuccessColor: ui.ToAdaptiveColor(cfg.UI().SuccessColor()),
			ErrorColor:   ui.ToAdaptiveColor(cfg.UI().ErrorColor()),
			MutedColor:   ui.ToAdaptiveColor(cfg.UI().MutedColor()),
			ShortToolbox: cfg.UI().ShortToolbox(),
		}

		width, height, _ := term.GetSize(int(os.Stdout.Fd()))
		m := history.NewModel(sess.Messages, sess.ToolDisplays, themeCfg, cfg.UI().ChatWindowWidth(), width, height)
		// Enable mouse input so the viewport can scroll with the wheel/trackpad.
		p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

		if _, err := p.Run(); err != nil {
			return fmt.Errorf("failed to run history viewer: %w", err)
		}

		return nil
	},
}
