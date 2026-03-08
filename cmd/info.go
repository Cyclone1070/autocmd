package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/auth"
	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(infoCmd)
}

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show information about the current configuration and state",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		appState, err := state.Load()
		if err != nil {
			return err
		}
		authMgr, err := buildAuthManager(cfg)
		if err != nil {
			return err
		}

		return runInfo(cmd, cfg, appState, authMgr)
	},
}

func runInfo(cmd *cobra.Command, cfg *config.Config, appState *state.State, authMgr *auth.Manager) error {
	llmRegistry := buildLLMRegistry()

	// Authorized Providers
	var authorized []string
	for _, pID := range llmRegistry.ListProviders() {
		if cred := resolveCredential(authMgr, pID); cred != nil {
			authorized = append(authorized, fmt.Sprintf("%s (%s)", pID, cred.Type))
		}
	}

	// Model Section
	if appState.Model != "" {
		cmd.Printf("\033[1m%-22s\033[0m %s\n", "Model:", appState.Model)
	}

	var sessMessages domain.Messages
	if appState.CurrentSessionID != "" {
		store, err := buildSessionStore(cfg)
		if err == nil {
			sess, err := store.Get(appState.CurrentSessionID)
			if err == nil {
				display := sess.Name
				if display == "" {
					display = sess.ID
				}
				cmd.Printf("\033[1m%-22s\033[0m %s\n", "Current Session:", display)
				sessMessages = sess.Messages
			} else {
				cmd.Printf("\033[1m%-22s\033[0m %s (not found)\n", "Current Session:", appState.CurrentSessionID)
			}
		}
	} else {
		cmd.Printf("\033[1m%-22s\033[0m %s\n", "Current Session:", "none")
	}

	// Optional LLM Info (if authed)
	providerID := strings.SplitN(appState.Model, domain.ModelIDSeparator, 2)[0]
	if cred := resolveCredential(authMgr, providerID); cred != nil && appState.Model != "" {
		ctx := context.Background()
		llmInstance, err := llmRegistry.Get(ctx, appState.Model, cred)
		if err == nil && llmInstance != nil {
			contextWindow := llmInstance.ContextWindow()
			if appState.CurrentSessionID != "" && len(sessMessages) > 0 {
				usage, err := llmInstance.ComputeTokens(ctx, sessMessages)
				if err == nil {
					cmd.Printf("\033[1m%-22s\033[0m %d tokens (%.1f%% of %d context)\n", "Session Usage:", usage, float64(usage)/float64(contextWindow)*100, contextWindow)
				}
			} else {
				cmd.Printf("\033[1m%-22s\033[0m %d tokens\n", "Context Window:", contextWindow)
			}
		}
	}

	if len(authorized) > 0 {
		cmd.Printf("\033[1m%-22s\033[0m %s\n", "Authorized Providers:", strings.Join(authorized, ", "))
	}

	return nil
}
