package cmd

import (
	"fmt"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/fs"
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
		bootstrapFS := fs.NewOSFileSystem(-1)

		configMgr := config.NewManager(bootstrapFS)
		cfg, err := configMgr.Load()
		if err != nil {
			return err
		}

		store, err := buildSessionStore(cfg, bootstrapFS)
		if err != nil {
			return err
		}

		sess, err := store.Create()
		if err != nil {
			return err
		}

		stateMgr := state.NewManager(bootstrapFS)
		appState, err := stateMgr.Load()
		if err != nil {
			return err
		}
		appState.SetCurrentSessionID(sess.ID)
		if err := appState.Save(); err != nil {
			return err
		}

		fmt.Printf("Started new session\n")
		return nil
	},
}
