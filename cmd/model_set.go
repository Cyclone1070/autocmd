package cmd

import (
	"context"
	"fmt"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/spf13/cobra"
)

func init() {
	modelCmd.AddCommand(modelSetCmd)
}

var modelSetCmd = &cobra.Command{
	Use:   "set [model-id]",
	Short: "Set the default LLM model",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		ctx := context.Background()
		registry, err := buildLLMRegistry(ctx, cfg)
		if err != nil {
			return err
		}

		// Validate model ID
		_, err = registry.Get(ctx, id)
		if err != nil {
			return fmt.Errorf("invalid model: %w", err)
		}

		cfg.Model = id
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Printf("Model set to: %s\n", id)
		return nil
	},
}
