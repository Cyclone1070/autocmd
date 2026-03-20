package cmd

import (
	"fmt"

	"github.com/Cyclone1070/iav/internal/ui"
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
		deps, err := Wire()
		if err != nil {
			return err
		}

		bus := workflow.NewEventBus()
		defer bus.Close()

		done := workflow.RunModelPicker(cmd.Context(), &workflow.ModelPickerDeps{
			Bus:      bus,
			Registry: deps.LLMRegistry,
			State:    deps.State,
		})

		themeCfg := ui.ThemeConfig{
			PrimaryColor: ui.ToAdaptiveColor(deps.Config.UI().PrimaryColor()),
			SuccessColor: ui.ToAdaptiveColor(deps.Config.UI().SuccessColor()),
			ErrorColor:   ui.ToAdaptiveColor(deps.Config.UI().ErrorColor()),
			MutedColor:   ui.ToAdaptiveColor(deps.Config.UI().MutedColor()),
			ShortToolbox: deps.Config.UI().ShortToolbox(),
		}
		theme := ui.NewTheme(themeCfg)

		m := model_picker.NewModel(bus, theme)
		p := tea.NewProgram(m)

		if _, err := p.Run(); err != nil {
			return fmt.Errorf("picker failed: %w", err)
		}

		if err := <-done; err != nil {
			return err
		}

		return nil
	},
}
