package cmd

import (
	"context"
	"fmt"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/Cyclone1070/iav/internal/ui/picker"
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
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		authMgr, err := buildAuthManager(cfg)
		if err != nil {
			return err
		}

		appState, err := state.Load()
		if err != nil {
			return err
		}

		ctx := context.Background()
		registry := buildLLMRegistry(authMgr)

		models, err := registry.List(ctx)
		if err != nil {
			return fmt.Errorf("list models: %w", err)
		}

		var items []picker.Item
		for _, mi := range models {
			items = append(items, picker.Item{
				ID:     mi.ID,
				Label:  mi.DisplayName,
				Detail: mi.ID,
				Active: mi.ID == appState.Model,
			})
		}

		pickerCfg := picker.Config{
			Title: "MODELS",
			Items: items,
		}

		m := picker.NewPicker(pickerCfg)
		p := tea.NewProgram(m)

		if _, err := p.Run(); err != nil {
			return fmt.Errorf("picker failed: %w", err)
		}

		if selected, ok := m.Selected(); ok {
			appState.Model = selected.ID
			if err := state.Save(appState); err != nil {
				return fmt.Errorf("failed to save state: %w", err)
			}
			fmt.Printf("\nSelected model: %s\n", selected.ID)
		}

		return nil
	},
}
