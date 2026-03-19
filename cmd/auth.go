package cmd

import (
	authui "github.com/Cyclone1070/iav/internal/ui/auth"
	"github.com/Cyclone1070/iav/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
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
		deps, err := Wire()
		if err != nil {
			return err
		}

		wf := workflow.NewAuthWorkflow(deps.LLMRegistry, deps.AuthManager, deps.State)
		m := authui.NewModel(wf)
		p := tea.NewProgram(m)

		if _, err := p.Run(); err != nil {
			return err
		}

		return m.Err()
	},
}
