package cmd

import (
	"fmt"
	"os"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/ui/history"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func init() {
	rootCmd.AddCommand(historyCmd)
}

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Print chat history of the current session",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		store, err := buildSessionStore(cfg)
		if err != nil {
			return err
		}

		var sessionID string
		if cfg.Session.CurrentSessionID != "" {
			sessionID = cfg.Session.CurrentSessionID
		} else {
			// Fallback to most recent session if none active
			summaries, err := store.List()
			if err != nil {
				return err
			}
			if len(summaries) == 0 {
				fmt.Println("No sessions found.")
				return nil
			}
			sessionID = summaries[0].ID
		}

		session, err := store.Get(sessionID)
		if err != nil {
			return fmt.Errorf("failed to load session %s: %w", sessionID, err)
		}

		width, height, err := term.GetSize(int(os.Stdout.Fd()))
		if err != nil {
			width = cfg.UI.ChatWindowWidth
			height = 20
		}

		m := history.NewModel(session.Messages, cfg.UI, width, height)
		p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
		if _, err := p.Run(); err != nil {
			return fmt.Errorf("failed to run history viewer: %w", err)
		}

		return nil
	},
}
