package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Cyclone1070/iav/internal/eventbus"
	"github.com/Cyclone1070/iav/internal/fs"
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

	theme := newTheme(deps.Config.UI())

	m := session_picker.NewModel(bus, theme)
	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("picker failed: %w", err)
	}

	res := <-done
	if res.Err != nil && !errors.Is(res.Err, context.Canceled) {
		return res.Err
	}

	return handleSwitchResult(res.SwitchCwd, os.Stdout)
}

func handleSwitchResult(switchCwd string, w io.Writer) error {
	if switchCwd != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "To continue this session, switch to its directory:")
		fmt.Fprintf(w, "  cd %s\n", switchCwd)
		fmt.Fprintln(w)
	}
	return nil
}

func getWorkingDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}

	dir := filepath.Clean(wd)
	for {
		gitDir := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitDir); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return wd
}
