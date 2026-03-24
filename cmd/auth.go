package cmd

import (
	"fmt"

	"github.com/Cyclone1070/iav/internal/eventbus"
	"github.com/Cyclone1070/iav/internal/ui"
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

		bus := eventbus.New()
		defer bus.Close()

		done := workflow.RunAuth(cmd.Context(), &workflow.AuthDeps{
			Bus:      bus,
			Registry: deps.LLMRegistry,
			AuthMgr:  deps.AuthManager,
			OAuthMgr: deps.OAuthManager,
			State:    deps.State,
		})

		themeCfg := ui.ThemeConfig{
			PrimaryColor: ui.ToAdaptiveColor(deps.Config.UI().PrimaryColor()),
			SuccessColor: ui.ToAdaptiveColor(deps.Config.UI().SuccessColor()),
			ErrorColor:   ui.ToAdaptiveColor(deps.Config.UI().ErrorColor()),
			MutedColor:   ui.ToAdaptiveColor(deps.Config.UI().MutedColor()),
			ShortToolbox: deps.Config.UI().ShortToolbox(),
		}
		theme := ui.NewTheme(themeCfg)

		m := authui.NewModel(bus, theme)
		p := tea.NewProgram(m)

		if _, err := p.Run(); err != nil {
			return fmt.Errorf("auth manager failed: %w", err)
		}

		if err := <-done; err != nil {
			return err
		}

		return nil
	},
}
