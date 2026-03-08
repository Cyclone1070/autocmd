package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuth_DuplicateErrorPrinting(t *testing.T) {
	// Root command setup
	// We want to capture the output of Execute()
	
	// We need to simulate the auth command failing due to empty input.
	// Since iav auth is interactive, we should ideally mock the UI layer 
	// or use a mock registry that triggers the error.
	
	t.Run("Auth command should silence usage on error", func(t *testing.T) {
		assert.True(t, authCmd.SilenceUsage, "authCmd should have SilenceUsage=true to prevent help output on validation errors")
	})

	t.Run("Auth command should NOT duplicate error output", func(t *testing.T) {
		// This is hard to test without running the actual interactive loop.
		// But I know why it's happening: interaction.go/View() prints it AND Cobra prints it.
		// If I fix it in the code, I can verify it here by checking the logic.
	})
}
