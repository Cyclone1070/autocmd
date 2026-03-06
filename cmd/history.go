package cmd

import (
	"fmt"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/Cyclone1070/iav/internal/ui/history"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"os"
)

func init() {
	rootCmd.AddCommand(historyCmd)
}

var historyCmd = &cobra.Command{
	Use:   "history [session_id]",
	Short: "View chat history for a session",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		appState, err := state.Load()
		if err != nil {
			return err
		}

		var sessionID string
		if len(args) > 0 {
			sessionID = args[0]
		} else {
			sessionID = appState.CurrentSessionID
		}

		store, err := buildSessionStore(cfg)
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

		width, height, _ := term.GetSize(int(os.Stdout.Fd()))
		m := history.NewModel(sess.Messages, sess.ToolDisplays, cfg.UI, width, height)
		p := tea.NewProgram(m, tea.WithAltScreen())

		if _, err := p.Run(); err != nil {
			return fmt.Errorf("failed to run history viewer: %w", err)
		}

		return nil
	},
}
