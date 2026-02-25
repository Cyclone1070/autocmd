package cmd

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/spf13/cobra"
)

func init() {
	sessionCmd.AddCommand(sessionListCmd)
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved conversation sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		store, err := buildSessionStore(cfg)
		if err != nil {
			return err
		}

		summaries, err := store.List()
		if err != nil {
			return err
		}

		fmt.Println("Saved Sessions:")
		fmt.Println(strings.Repeat("-", 40))
		for _, s := range summaries {
			fmt.Printf("- %s (Messages: %d, Updated: %s)\n", s.ID, s.MessageCount, s.Updated.Format("2006-01-02 15:04:05"))
		}

		return nil
	},
}
