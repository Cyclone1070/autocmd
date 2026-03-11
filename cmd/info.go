package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/auth"
	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/fs"
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

		authMgr, err := buildAuthManager(cfg)
		if err != nil {
			return err
		}

		return runInfo(cmd, bootstrapFS, cfg, appState, authMgr)
	},
}

func runInfo(cmd *cobra.Command, bootstrapFS fs.FileSystem, cfg *config.Config, appState *state.State, authMgr *auth.Manager) error {
	ctx := context.Background()
	llmRegistry := buildLLMRegistry(authMgr)

	// Authorized Providers
	var authorized []string
	providers, err := llmRegistry.ListProviders(ctx)
	if err != nil {
		return fmt.Errorf("list providers: %w", err)
	}
	for _, p := range providers {
		if p.Credential != nil {
			authorized = append(authorized, fmt.Sprintf("%s (%s)", p.ID, p.Credential.Type))
		}
	}

	// Model Section
	if appState.Model() != "" {
		cmd.Printf("\033[1m%-22s\033[0m %s\n", "Model:", appState.Model())
	}

	var sessMessages domain.Messages
	if appState.CurrentSessionID() != "" {
		store, err := buildSessionStore(cfg, bootstrapFS)
		if err == nil {
			sess, err := store.Get(appState.CurrentSessionID())
			if err == nil {
				display := sess.Name
				if display == "" {
					display = sess.ID
				}
				cmd.Printf("\033[1m%-22s\033[0m %s\n", "Current Session:", display)
				sessMessages = sess.Messages
			} else {
				cmd.Printf("\033[1m%-22s\033[0m %s (not found)\n", "Current Session:", appState.CurrentSessionID())
			}
		}
	} else {
		cmd.Printf("\033[1m%-22s\033[0m %s\n", "Current Session:", "none")
	}

	// Optional LLM Info (if authed)
	if appState.Model() != "" {
		llmInstance, err := llmRegistry.Get(ctx, appState.Model())
		if err == nil && llmInstance != nil {
			contextWindow := llmInstance.ContextWindow()
			if appState.CurrentSessionID() != "" && len(sessMessages) > 0 {
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
