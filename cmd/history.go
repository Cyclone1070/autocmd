package cmd

import (
	"fmt"
	"os"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/ui/history"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
		if len(args) > 0 {
			sessionID = args[0]
		} else {
			state, err := config.LoadState()
			if err != nil {
				return err
			}
			sessionID = state.CurrentSessionID
		}

		if sessionID == "" {
			// Fallback: try most recent session if no active one
			summaries, err := store.List()
			if err != nil || len(summaries) == 0 {
				return fmt.Errorf("no current session found and no history available")
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

		isDark := lipgloss.HasDarkBackground()
		m := history.NewModel(session.Messages, session.ToolDisplays, cfg.UI, width, height, history.WithIsDark(isDark))
		p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
		if _, err := p.Run(); err != nil {
			return fmt.Errorf("failed to run history viewer: %w", err)
		}

		return nil
	},
}
