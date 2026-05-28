package cmd

import (
	"context"

	"github.com/Cyclone1070/iav/internal/eventbus"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/Cyclone1070/iav/internal/ui/info"
	"github.com/Cyclone1070/iav/internal/workflow"
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

	fileSystem := fs.NewOSFileSystem(-1)
	store, err := buildSessionStore(fileSystem)
	if err != nil {
		return err
	}

	workingDir := getWorkingDir()
	sess, err := workflow.ResolveWorkspaceSession(store, workingDir)
	if err != nil {
		return err
	}

	done := workflow.RunInfo(ctx, &workflow.InfoDeps{
		Bus:              bus,
		ProviderRegistry: deps.ProviderRegistry,
		LLMRegistry:      deps.LLMRegistry,
		State:            deps.State,
		Store:            store,
		SessionID:        sess.ID,
	})

	theme := newTheme(deps.Config.UI())

	uiModel := info.NewModel(bus, theme)
	p := tea.NewProgram(uiModel)
	if _, err := p.Run(); err != nil {
		return err
	}

	return <-done
}
