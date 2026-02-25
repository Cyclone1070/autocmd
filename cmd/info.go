package cmd

import (
	"fmt"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(infoCmd)
}

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Display current configuration and state",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		fmt.Printf("Model:   %s\n", cfg.Model)
		fmt.Printf("Active Session:  %s\n", cfg.Session.CurrentSessionID)
		fmt.Printf("Session Storage: %s\n", cfg.Session.StorageDir)
		fmt.Printf("Max Iterations:  %d\n", cfg.Tools.MaxIterations)
		fmt.Printf("Max File Size:   %d bytes\n", cfg.Tools.MaxFileSize)

		return nil
	},
}
