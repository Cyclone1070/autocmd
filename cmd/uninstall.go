package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Cyclone1070/autocmd/internal/domain"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(uninstallCmd)
}

var uninstallCmd = &cobra.Command{
	Use:          "uninstall",
	Short:        "Remove AutoCmd and clean up configuration files",
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, _ []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("home directory: %w", err)
		}
		configDir := filepath.Join(home, domain.ConfigBaseDir, domain.AppName)

		fmt.Printf("Removing %s ...\n", configDir)
		if err := os.RemoveAll(configDir); err != nil {
			return fmt.Errorf("remove %s: %w", configDir, err)
		}
		fmt.Println("Done. AutoCmd configuration has been removed.")
		return nil
	},
}
