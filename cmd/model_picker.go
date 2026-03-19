package cmd

import (
	"fmt"

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

		wf := workflow.NewModelPickerWorkflow(deps.LLMRegistry, deps.State)

		m := model_picker.NewModel(wf)
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
