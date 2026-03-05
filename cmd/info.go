package cmd

import (
	"context"
	"fmt"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(infoCmd)
}

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Display current configuration and state",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// 1. Model
		cmd.Printf("Model:          %s\n", cfg.Model)

		// 2. Session Name
		store, err := buildSessionStore(cfg)
		if err != nil {
			return fmt.Errorf("failed to build session store: %w", err)
		}

		sessionName := "None"
		var sessMessages []domain.Message
		if cfg.Session.CurrentSessionID != "" {
			sess, err := store.Get(cfg.Session.CurrentSessionID)
			if err == nil {
				sessionName = sess.Name
				if sessionName == "" {
					sessionName = "Untitled (" + sess.ID[:8] + ")"
				}
				sessMessages = sess.Messages
			}
		}
		cmd.Printf("Session Name:   %s\n", sessionName)

		// 3. Context Window
		llmRegistry, err := buildLLMRegistry(ctx, cfg)
		if err != nil {
			return fmt.Errorf("failed to build LLM registry: %w", err)
		}

		llmInstance, err := llmRegistry.Get(ctx, cfg.Model)
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
