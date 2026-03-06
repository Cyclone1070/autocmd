package cmd

import (
	"fmt"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/spf13/cobra"
)

func init() {
	sessionCmd.AddCommand(newCmd)
}

var newCmd = &cobra.Command{
	Use:   "new",
	Short: "Start a new chat session",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		store, err := buildSessionStore(cfg)
		if err != nil {
			return err
		}

		sess, err := store.Create()
		if err != nil {
			return err
		}

		appState, err := state.Load()
		if err != nil {
			return err
		}
		appState.CurrentSessionID = sess.ID
		if err := state.Save(appState); err != nil {
			return err
		}

		fmt.Printf("Started new session: %s\n", sess.ID)
		return nil
	},
}
