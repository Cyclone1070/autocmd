package cmd

import (
	"fmt"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/Cyclone1070/iav/internal/ui/session_picker"
	"github.com/Cyclone1070/iav/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(sessionCmd)
	sessionCmd.AddCommand(listCmd)
}

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage conversation sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSessionPicker(cmd, args)
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List and manage chat sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSessionPicker(cmd, args)
	},
}

func runSessionPicker(cmd *cobra.Command, args []string) error {
	bootstrapFS := fs.NewOSFileSystem(-1)

	configMgr := config.NewManager(bootstrapFS)
	cfg, err := configMgr.Load()
	if err != nil {
		return err
	}

	stateMgr := state.NewManager(bootstrapFS)
	appState, err := stateMgr.Load()
	if err != nil {
		return err
	}

	store, err := buildSessionStore(cfg, bootstrapFS)
	if err != nil {
		return err
	}

	wf := workflow.NewSessionPickerWorkflow(store, appState)
	m := session_picker.NewModel(wf)
	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("picker failed: %w", err)
	}

	if err := m.Err(); err != nil {
		return err
	}

	return nil
}
