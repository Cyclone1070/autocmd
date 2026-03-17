package cmd

import (
	"context"
	"fmt"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/Cyclone1070/iav/internal/ui/model_picker"
	"github.com/Cyclone1070/iav/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(modelCmd)
}

var modelCmd = &cobra.Command{
	Use:   "model",
	Short: "Choose the default LLM model",
	RunE: func(cmd *cobra.Command, args []string) error {
		bootstrapFS := fs.NewOSFileSystem(-1)

		configMgr := config.NewManager(bootstrapFS)
		cfg, err := configMgr.Load()
		if err != nil {
			return err
		}

		authMgr, err := buildAuthManager(cfg)
		if err != nil {
			return err
		}

		stateMgr := state.NewManager(bootstrapFS)
		appState, err := stateMgr.Load()
		if err != nil {
			return err
		}

		ctx := context.Background()
		registry := buildLLMRegistry(authMgr)

		wf := workflow.NewModelPickerWorkflow(registry, appState)
		res, err := wf.PrepareSelection(ctx)
		if err != nil {
			return err
		}

		m := model_picker.NewModel(res, wf)
		p := tea.NewProgram(m)

		if _, err := p.Run(); err != nil {
			return fmt.Errorf("picker failed: %w", err)
		}

		if err := m.Err(); err != nil {
			return err
		}

		return nil
	},
}
