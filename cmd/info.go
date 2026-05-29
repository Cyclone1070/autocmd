package cmd

import (
	"context"

	"github.com/Cyclone1070/autocmd/internal/eventbus"
	"github.com/Cyclone1070/autocmd/internal/ui/info"
	"github.com/Cyclone1070/autocmd/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(infoCmd)
}

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show information about the current configuration and state",
	RunE: func(cmd *cobra.Command, _ []string) error {
		deps, err := Wire()
		if err != nil {
			return err
		}

		return runInfo(cmd.Context(), deps)
	},
}

func runInfo(ctx context.Context, deps *Deps) error {
	bus := eventbus.New()
	defer bus.Close()

	workingDir := getWorkingDir()

	var sessionID string
	active, err := deps.SessionStore.FindActiveForDir(workingDir)
	if err == nil && active != nil {
		sessionID = active.ID
	}

	done := workflow.RunInfo(ctx, &workflow.InfoDeps{
		Bus:              bus,
		ProviderRegistry: deps.ProviderRegistry,
		LLMRegistry:      deps.LLMRegistry,
		State:            deps.State,
		Store:            deps.SessionStore,
		SessionID:        sessionID,
	})

	theme := newTheme(deps.Config.UI())

	uiModel := info.NewModel(bus, theme)
	p := tea.NewProgram(uiModel)
	if _, err := p.Run(); err != nil {
		return err
	}

	return <-done
}
