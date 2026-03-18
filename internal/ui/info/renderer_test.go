package info

import (
	"bytes"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestInfoRenderer_Render(t *testing.T) {
	t.Run("Full Success Scenario", func(t *testing.T) {
		data := &domain.SystemSnapshot{
			Model:          "google/gemini-pro",
			SessionDisplay: "Test Session",
			SessionTokens:  100,
			ContextWindow:  128000,
			Authorized:     []string{"google (env)"},
		}

		b := &bytes.Buffer{}
		cmd := &cobra.Command{}
		cmd.SetOut(b)

		renderer := &InfoRenderer{}
		renderer.Render(cmd, data)

		output := b.String()
		assert.Contains(t, output, "Model:")
		assert.Contains(t, output, "google/gemini-pro")
		assert.Contains(t, output, "Current Session:")
		assert.Contains(t, output, "Test Session")
		assert.Contains(t, output, "Session Usage:")
		assert.Contains(t, output, "100 tokens")
		assert.Contains(t, output, "0.1% of 128000 context")
		assert.Contains(t, output, "Authorized Providers:")
		assert.Contains(t, output, "google (env)")
	})

	t.Run("Minimal Scenario", func(t *testing.T) {
		data := &domain.SystemSnapshot{
			SessionDisplay: "none",
		}

		b := &bytes.Buffer{}
		cmd := &cobra.Command{}
		cmd.SetOut(b)

		renderer := &InfoRenderer{}
		renderer.Render(cmd, data)

		output := b.String()
		assert.Contains(t, output, "Current Session:")
		assert.Contains(t, output, "none")
		assert.NotContains(t, output, "Model:")
		assert.NotContains(t, output, "Authorized Providers:")
	})
}
