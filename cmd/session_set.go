package cmd

import (
	"fmt"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/spf13/cobra"
)

func init() {
	sessionCmd.AddCommand(sessionSetCmd)
}

var sessionSetCmd = &cobra.Command{
	Use:   "set [session-id]",
	Short: "Set the active conversation session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		store, err := buildSessionStore(cfg)
		if err != nil {
			return err
		}

		// Validate session exists
		_, err = store.Get(id)
		if err != nil {
			return fmt.Errorf("session not found: %s", id)
		}

		cfg.Session.CurrentSessionID = id
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Printf("Active session set to: %s\n", id)
		return nil
	},
}
