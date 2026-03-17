package cmd

import (
	"context"

	"github.com/Cyclone1070/iav/internal/auth"
	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/Cyclone1070/iav/internal/ui/info"
	"github.com/Cyclone1070/iav/internal/workflow"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(infoCmd)
}

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show information about the current configuration and state",
	RunE: func(cmd *cobra.Command, args []string) error {
		bootstrapFS := fs.NewOSFileSystem(-1)

		configMgr := config.NewManager(bootstrapFS)
		cfg, err := configMgr.Load()
		if err != nil {
			return err
		}

		stateMgr := state.NewManager(bootstrapFS)
		appState, err := stateMgr.Load()
		if err != nil {
			return err
		}

		authMgr, err := buildAuthManager(cfg)
		if err != nil {
			return err
		}

		return runInfo(cmd, bootstrapFS, cfg, appState, authMgr)
	},
}

func runInfo(cmd *cobra.Command, bootstrapFS fs.FileSystem, cfg *config.Config, appState *state.State, authMgr *auth.Manager) error {
	ctx := context.Background()
	llmRegistry := buildLLMRegistry(authMgr)

	store, err := buildSessionStore(cfg, bootstrapFS)
	if err != nil {
		return err
	}

	wf := workflow.NewInfoWorkflow(llmRegistry, appState, store)
	res, err := wf.Run(ctx)
	if err != nil {
		return err
	}

	renderer := &info.InfoRenderer{}
	renderer.Render(cmd, res)

	return nil
}
