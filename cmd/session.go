package cmd

import (
	"fmt"

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
	deps, err := Wire()
	if err != nil {
		return err
	}

	wf := workflow.NewSessionPickerWorkflow(deps.SessionStore, deps.State)
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
