package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/spf13/cobra"
)

func init() {
	modelCmd.AddCommand(modelListCmd)
}

var modelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available LLM models",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		ctx := context.Background()
		registry, err := buildLLMRegistry(ctx, cfg)
		if err != nil {
			return err
		}

		models, err := registry.List(ctx)
		if err != nil {
			return err
		}

		fmt.Println("Available Models:")
		fmt.Println(strings.Repeat("-", 40))
		for _, m := range models {
			current := ""
			if m.ID == cfg.Model {
				current = " (current)"
			}
			fmt.Printf("- %-30s %s%s\n", m.ID, m.DisplayName, current)
		}

		return nil
	},
}
