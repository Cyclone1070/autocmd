package info

import (
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/spf13/cobra"
)

// InfoRenderer handles formatting and printing of info data to the terminal.
type InfoRenderer struct{}

// Render prints the info data to the provided cobra command's output.
func (r *InfoRenderer) Render(cmd *cobra.Command, data *domain.SystemSnapshot) {
	// Model Section
	if data.Model != "" {
		cmd.Printf("\033[1m%-22s\033[0m %s\n", "Model:", data.Model)
	}

	// Session Section
	cmd.Printf("\033[1m%-22s\033[0m %s\n", "Current Session:", data.SessionDisplay)

	// Usage Section (only if model and tokens/window are present)
	if data.Model != "" && data.ContextWindow > 0 {
		if data.SessionTokens > 0 {
			usagePct := float64(data.SessionTokens) / float64(data.ContextWindow) * 100
			cmd.Printf("\033[1m%-22s\033[0m %d tokens (%.1f%% of %d context)\n", "Session Usage:", data.SessionTokens, usagePct, data.ContextWindow)
		} else {
			cmd.Printf("\033[1m%-22s\033[0m %d tokens\n", "Context Window:", data.ContextWindow)
		}
	}

	// Authorized Providers Section
	if len(data.Authorized) > 0 {
		cmd.Printf("\033[1m%-22s\033[0m %s\n", "Authorized Providers:", strings.Join(data.Authorized, ", "))
	}
}
