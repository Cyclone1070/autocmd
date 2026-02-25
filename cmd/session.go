package cmd

import (
	"fmt"

	"github.com/Cyclone1070/iav/internal/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(sessionCmd)
}

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage conversation sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		store, err := buildSessionStore(cfg)
		if err != nil {
			return err
		}

		summaries, err := store.List()
		if err != nil {
			return err
		}

		m := newSessionPickerModel(cfg, store, summaries)
		p := tea.NewProgram(m)
		if _, err := p.Run(); err != nil {
			return fmt.Errorf("picker failed: %w", err)
		}

		return nil
	},
}
