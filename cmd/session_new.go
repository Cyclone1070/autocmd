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

		state, err := config.LoadState()
		if err != nil {
			return err
		}

		store, err := buildSessionStore(cfg)
		if err != nil {
			return err
		}

		// Check if current session is already blank
		if state.CurrentSessionID != "" {
			sess, err := store.Get(state.CurrentSessionID)
			if err == nil && len(sess.Messages) == 0 {
				fmt.Println("Already on a blank session.")
				return nil
			}
		}

		// Create new session
		sess, err := store.Create()
		if err != nil {
			return err
		}

		state.CurrentSessionID = sess.ID
		if err := config.SaveState(state); err != nil {
			return fmt.Errorf("failed to save state: %w", err)
		}

		fmt.Printf("Created new session: %s\n", sess.ID)
		return nil
	},
}
