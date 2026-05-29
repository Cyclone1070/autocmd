package cmd

import (
	"fmt"

	"github.com/Cyclone1070/autocmd/internal/workflow"
	"github.com/spf13/cobra"
)

func init() {
	sessionCmd.AddCommand(newCmd)
}

var newCmd = &cobra.Command{
	Use:   "new",
	Short: "Start a new chat session",
	RunE: func(_ *cobra.Command, _ []string) error {
		deps, err := Wire()
		if err != nil {
			return err
		}

		workingDir := getWorkingDir()
		if _, err := workflow.CreateSession(deps.SessionStore, workingDir); err != nil {
			return err
		}

		theme := newTheme(deps.Config.UI())
		fmt.Printf("\nSelected session: %s\n\n", theme.Success("Untitled"))
		return nil
	},
}
