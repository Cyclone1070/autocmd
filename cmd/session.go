package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

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
	RunE: func(cmd *cobra.Command, _ []string) error {
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
	store, err := buildSessionStore(fileSystem)
	if err != nil {
		return err
	}

	workingDir := getWorkingDir()

	done := workflow.RunSessionPicker(ctx, &workflow.SessionPickerDeps{
		Bus:        bus,
		Store:      store,
		WorkingDir: workingDir,
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

	res := <-done
	if res.Err != nil && !errors.Is(res.Err, context.Canceled) {
		return res.Err
	}

	if res.SwitchCwd != "" {
		created := false
		if _, err := os.Stat(res.SwitchCwd); os.IsNotExist(err) {
			if err := os.MkdirAll(res.SwitchCwd, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
			created = true
		}

		if created {
			fmt.Printf("Created directory: %s\n", res.SwitchCwd)
		}
		fmt.Printf("Workspace directory changed to: %s\n", res.SwitchCwd)

		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}

		subshell := exec.Command(shell)
		subshell.Dir = res.SwitchCwd
		subshell.Stdin = os.Stdin
		subshell.Stdout = os.Stdout
		subshell.Stderr = os.Stderr
		if err := subshell.Run(); err != nil {
			return fmt.Errorf("failed to spawn subshell: %w", err)
		}
	}

	return nil
}
