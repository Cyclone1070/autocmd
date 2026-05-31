package cmd

import (
	"fmt"
	"os"

	"github.com/Cyclone1070/autocmd/internal/eventbus"
	"github.com/Cyclone1070/autocmd/internal/ui/history"
	"github.com/Cyclone1070/autocmd/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func init() {
	rootCmd.AddCommand(historyCmd)
}

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "View chat history for the current session",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		deps, err := Wire()
		if err != nil {
			return err
		}

		bus := eventbus.New()
		defer bus.Close()

		workingDir := getWorkingDir()

		var sessionID string
		active, err := deps.SessionStore.FindActiveForDir(workingDir)
		if err == nil && active != nil {
			sessionID = active.ID
		}

		done := workflow.RunHistory(cmd.Context(), &workflow.HistoryDeps{
			Store:     deps.SessionStore,
			SessionID: sessionID,
		}, bus)

		theme := newTheme(deps.Config.UI())

		chatWindowWidth := deps.Config.UI().ChatWindowWidth()
		width, height, _ := term.GetSize(int(os.Stdout.Fd()))
		if chatWindowWidth <= 0 {
			if width > 0 {
				chatWindowWidth = width
			} else {
				chatWindowWidth = 80
			}
		}
		m := history.NewModel(bus, theme, chatWindowWidth, deps.Config.UI().BashOutputHeight(), width, height)
		p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

		if _, err := p.Run(); err != nil {
			return fmt.Errorf("failed to run history viewer: %w", err)
		}

		return <-done
	},
}
