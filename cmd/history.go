package cmd

import (
	"fmt"
	"os"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/Cyclone1070/iav/internal/ui/history"
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

		var sessionID string
		if len(args) > 0 {
			sessionID = args[0]
		} else {
			sessionID = appState.CurrentSessionID()
		}

		store, err := buildSessionStore(cfg, bootstrapFS)
		if err != nil {
			return err
		}

		if sessionID == "" {
			summaries, err := store.List()
			if err != nil || len(summaries) == 0 {
				return fmt.Errorf("no current session found and no history available")
			}
			sessionID = summaries[0].ID
		}

		sess, err := store.Get(sessionID)
		if err != nil {
			return fmt.Errorf("failed to load session %s: %w", sessionID, err)
		}

		themeCfg := ui.ThemeConfig{
			PrimaryColor: ui.ToAdaptiveColor(cfg.UI().PrimaryColor()),
			SuccessColor: ui.ToAdaptiveColor(cfg.UI().SuccessColor()),
			ErrorColor:   ui.ToAdaptiveColor(cfg.UI().ErrorColor()),
			MutedColor:   ui.ToAdaptiveColor(cfg.UI().MutedColor()),
			ShortToolbox: cfg.UI().ShortToolbox(),
		}

		width, height, _ := term.GetSize(int(os.Stdout.Fd()))
		m := history.NewModel(sess.Messages, sess.ToolDisplays, themeCfg, cfg.UI().ChatWindowWidth(), width, height)
		p := tea.NewProgram(m, tea.WithAltScreen())

		if _, err := p.Run(); err != nil {
			return fmt.Errorf("failed to run history viewer: %w", err)
		}

		return nil
	},
}
