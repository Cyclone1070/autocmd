package cmd

import (
	"fmt"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/Cyclone1070/iav/internal/ui"
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

		stateMgr := state.NewManager(bootstrapFS)
		appState, err := stateMgr.Load()
		if err != nil {
			return err
		}

		store, err := buildSessionStore(bootstrapFS)
		if err != nil {
			return err
		}

		if _, err := workflow.CreateSession(store, appState); err != nil {
			return err
		}

		themeCfg := ui.ThemeConfig{
			PrimaryColor:   ui.ToAdaptiveColor(cfg.UI().PrimaryColor()),
			SuccessColor:   ui.ToAdaptiveColor(cfg.UI().SuccessColor()),
			ErrorColor:     ui.ToAdaptiveColor(cfg.UI().ErrorColor()),
			MutedColor:     ui.ToAdaptiveColor(cfg.UI().MutedColor()),
			ShortToolBlock: cfg.UI().ShortToolBlock(),
		}
		theme := ui.NewTheme(themeCfg)
		fmt.Printf("\nSelected session: %s\n\n", theme.Success("(new session)"))
		return nil
	},
}
