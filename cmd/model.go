package cmd

import (
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(modelCmd)
}

var modelCmd = &cobra.Command{
	Use:   "model",
	Short: "Manage LLM models",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runModelPicker()
	},
}
