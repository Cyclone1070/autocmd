package cmd

import (
	"fmt"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/spf13/cobra"
)

func init() {
	sessionCmd.AddCommand(sessionNewCmd)
}

var sessionNewCmd = &cobra.Command{
	Use:   "new",
	Short: "Start a new blank conversation session",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		store, err := buildSessionStore(cfg)
		if err != nil {
			return err
		}

		// Check if we are already on a blank session
		if cfg.Session.CurrentSessionID != "" {
			curr, err := store.Get(cfg.Session.CurrentSessionID)
			if err == nil && curr.Name == "" && len(curr.Messages) == 0 {
				fmt.Println("Already on a blank session.")
				return nil
			}
		}

		// Look for an existing blank session to reuse
		blank, err := store.FindBlank()
		if err != nil {
			return err
		}

		var id string
		if blank != nil {
			id = blank.ID
			fmt.Printf("Switched to existing blank session: %s\n", id)
		} else {
			sess, err := store.Create()
			if err != nil {
				return err
			}
			id = sess.ID
			fmt.Printf("Created new blank session: %s\n", id)
		}

		cfg.Session.CurrentSessionID = id
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		return nil
	},
}
