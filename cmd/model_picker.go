package cmd

import (
	"fmt"

	"github.com/Cyclone1070/autocmd/internal/eventbus"
	"github.com/Cyclone1070/autocmd/internal/ui/model_picker"
	"github.com/Cyclone1070/autocmd/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(modelCmd)
}

var modelCmd = &cobra.Command{
	Use:   "model",
	Short: "Choose the default LLM model",
	RunE: func(cmd *cobra.Command, _ []string) error {
		deps, err := Wire()
		if err != nil {
			return err
		}

		bus := eventbus.New()
		defer bus.Close()

		done := workflow.RunModelPicker(cmd.Context(), &workflow.ModelPickerDeps{
			Bus:      bus,
			Registry: deps.LLMRegistry,
			State:    deps.State,
		})

		theme := newTheme(deps.Config.UI())

		m := model_picker.NewModel(bus, theme)
		p := tea.NewProgram(m)

		if _, err := p.Run(); err != nil {
			return fmt.Errorf("picker failed: %w", err)
		}

		return <-done
	},
}
