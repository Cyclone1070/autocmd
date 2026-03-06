package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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
	Short: "Display current configuration and state",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		appState, err := state.Load()
		if err != nil {
			return err
		}

		fmt.Printf("Model:   %s\n", appState.Model)
		fmt.Printf("Config:  %s\n", filepath.Join(os.Getenv("HOME"), ".config", "iav", "config.json"))
		fmt.Printf("State:   %s\n", filepath.Join(os.Getenv("HOME"), ".config", "iav", "state.json"))
		fmt.Printf("Storage: %s\n", cfg.Session.StorageDir)

		var sessMessages domain.Messages
		if appState.CurrentSessionID != "" {
			store, err := buildSessionStore(cfg)
			if err == nil {
				sess, err := store.Get(appState.CurrentSessionID)
				if err == nil {
					fmt.Printf("Current Session: %s (%d messages, last updated %s)\n",
						appState.CurrentSessionID,
						len(sess.Messages),
						sess.Updated.Format("Jan 02 15:04"))
					sessMessages = sess.Messages
				} else {
					fmt.Printf("Current Session: %s (not found)\n", appState.CurrentSessionID)
				}
			}
		} else {
			fmt.Println("Current Session: none")
		}

		ctx := context.Background()
		llmRegistry, err := buildLLMRegistry(ctx, cfg)
		if err != nil {
			return fmt.Errorf("failed to build LLM registry: %w", err)
		}

		llmInstance, err := llmRegistry.Get(ctx, appState.Model)
		if err != nil {
			return fmt.Errorf("failed to get LLM instance: %w", err)
		}

		contextWindow := llmInstance.ContextWindow()
		usedTokens := 0
		var computeErr error
		if len(sessMessages) > 0 {
			usedTokens, computeErr = llmInstance.ComputeTokens(ctx, sessMessages)
		}

		if computeErr != nil {
			cmd.Printf("Context Window: ?/%d tokens (token count failed)\n", contextWindow)
		} else {
			percentage := 0.0
			if contextWindow > 0 {
				percentage = float64(usedTokens) / float64(contextWindow) * 100
			}
			cmd.Printf("Context Window: %.1f%% used (%d/%d tokens)\n", percentage, usedTokens, contextWindow)
		}

		return nil
	},
}
