// Package cmd implements the command-line interface for the application.
package cmd

import (
	"fmt"

	"github.com/Cyclone1070/autocmd/internal/eventbus"
	authui "github.com/Cyclone1070/autocmd/internal/ui/auth"
	"github.com/Cyclone1070/autocmd/internal/workflow"
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
	RunE: func(cmd *cobra.Command, _ []string) error {
		deps, err := Wire()
		if err != nil {
			return err
		}

		bus := eventbus.New()
		defer bus.Close()

		done := workflow.RunAuth(cmd.Context(), &workflow.AuthDeps{
			Bus:      bus,
			Registry: deps.ProviderRegistry,
			AuthMgr:  deps.AuthManager,
			OAuthMgr: deps.OAuthManager,
			State:    deps.State,
		})

		theme := newTheme(deps.Config.UI())

		m := authui.NewModel(bus, theme)
		p := tea.NewProgram(m)

		if _, err := p.Run(); err != nil {
			return fmt.Errorf("auth manager failed: %w", err)
		}

		return <-done
	},
}
