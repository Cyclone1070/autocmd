package cmd

import (
	"context"
	"fmt"

	"github.com/Cyclone1070/iav/internal/eventbus"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/Cyclone1070/iav/internal/ui/session_picker"
	"github.com/Cyclone1070/iav/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(sessionCmd)
}

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage conversation sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := Wire()
		if err != nil {
			return err
		}
		return runSessionPicker(cmd.Context(), deps)
	},
}

func runSessionPicker(ctx context.Context, deps *Deps) error {
	bus := eventbus.New()
	defer bus.Close()

	fileSystem := fs.NewOSFileSystem(-1)
	store, err := buildSessionStore(deps.Config, fileSystem)
	if err != nil {
		return err
	}

	done := workflow.RunSessionPicker(ctx, &workflow.SessionPickerDeps{
		Bus:   bus,
		Store: store,
		State: deps.State,
	})

	themeCfg := ui.ThemeConfig{
		PrimaryColor:   ui.ToAdaptiveColor(deps.Config.UI().PrimaryColor()),
		SuccessColor:   ui.ToAdaptiveColor(deps.Config.UI().SuccessColor()),
		ErrorColor:     ui.ToAdaptiveColor(deps.Config.UI().ErrorColor()),
		MutedColor:     ui.ToAdaptiveColor(deps.Config.UI().MutedColor()),
		ShortToolBlock: deps.Config.UI().ShortToolBlock(),
	}
	theme := ui.NewTheme(themeCfg)

	m := session_picker.NewModel(bus, theme)
	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("picker failed: %w", err)
	}

	if err := <-done; err != nil {
		return err
	}

	return nil
}
