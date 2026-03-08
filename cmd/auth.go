package cmd

import (
	"github.com/Cyclone1070/iav/internal/config"
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
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		authMgr, err := buildAuthManager(cfg)
		if err != nil {
			return err
		}
		registry := buildLLMRegistry()
		s, err := state.Load()
		if err != nil {
			return err
		}
		return authui.Run(registry, authMgr, s)
	},
}
