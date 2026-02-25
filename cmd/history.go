package cmd

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/spf13/cobra"
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

		fmt.Printf("Session: %s (%s)\n", session.ID, session.Updated.Format("2006-01-02 15:04:05"))
		fmt.Println(strings.Repeat("-", 40))

		for _, msg := range session.Messages {
			role := strings.ToUpper(string(msg.Role))
			fmt.Printf("[%s]:\n%s\n\n", role, msg.Content)
			if len(msg.ToolCalls) > 0 {
				fmt.Printf("  (Tool Calls: %d)\n\n", len(msg.ToolCalls))
			}
		}

		return nil
	},
}
