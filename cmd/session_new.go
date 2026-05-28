package cmd

import (
	"fmt"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/Cyclone1070/iav/internal/workflow"
	"github.com/spf13/cobra"
)

func init() {
	sessionCmd.AddCommand(newCmd)
}

var newCmd = &cobra.Command{
	Use:   "new",
	Short: "Start a new chat session",
	RunE: func(_ *cobra.Command, _ []string) error {
		bootstrapFS := fs.NewOSFileSystem(-1)

		configMgr := config.NewManager(bootstrapFS)
		cfg, err := configMgr.Load()
		if err != nil {
			return err
		}

		store, err := buildSessionStore(bootstrapFS)
		if err != nil {
			return err
		}

		workingDir := getWorkingDir()
		if _, err := workflow.CreateSession(store, workingDir); err != nil {
			return err
		}

		theme := newTheme(cfg.UI())
		fmt.Printf("\nSelected session: %s\n\n", theme.Success("Untitled"))
		return nil
	},
}
