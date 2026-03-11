package cmd

import (
	"fmt"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/Cyclone1070/iav/internal/state"
	authui "github.com/Cyclone1070/iav/internal/ui/auth"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(authCmd)
}

var authCmd = &cobra.Command{
	Use:          "auth",
	Short:        "Manage authentication for LLM providers",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		bootstrapFS := fs.NewOSFileSystem(-1)

		configMgr := config.NewManager(bootstrapFS)
		cfg, err := configMgr.Load()
		if err != nil {
			return err
		}

		authMgr, err := buildAuthManager(cfg)
		if err != nil {
			return fmt.Errorf("build auth manager: %w", err)
		}

		registry := buildLLMRegistry(authMgr)

		stateMgr := state.NewManager(bootstrapFS)
		s, err := stateMgr.Load()
		if err != nil {
			return err
		}

		return authui.Run(registry, authMgr, s)
	},
}
