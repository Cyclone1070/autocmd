package prompt

import (
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/stretchr/testify/assert"
)

type mockSpinner struct{}

func (m *mockSpinner) Frame(tick int) string { return "*" }

type mockColor struct {
	light string
}

func (m mockColor) Light() string { return m.light }
func (m mockColor) Dark() string  { return m.light }

func TestThinkingRenderer_Styling(t *testing.T) {
	primary := ui.ToAdaptiveColor(mockColor{light: "#0000FF"})
	success := ui.ToAdaptiveColor(mockColor{light: "#00FF00"})
	errColor := ui.ToAdaptiveColor(mockColor{light: "#FF0000"})

	theme := ui.NewTheme(ui.ThemeConfig{
		PrimaryColor: primary,
		SuccessColor: success,
		ErrorColor:   errColor,
	})
	r := NewThinkingRenderer(theme)
	sp := &mockSpinner{}

	t.Run("StatusRunning", func(t *testing.T) {
		got := r.RenderThinking(ui.StatusRunning, time.Now(), 0, sp)
		assert.NotContains(t, got, "...", "Ellipsis should be removed")
		assert.Contains(t, got, "Thinking for")

		expectedLabel := theme.Primary("Thinking for 0s")
		assert.Contains(t, got, expectedLabel, "Running label should be Primary color")
		assert.Contains(t, got, "\n    ", "Thinking line should be inset by four spaces")
	})

	t.Run("StatusSuccess", func(t *testing.T) {
		got := r.RenderThinking(ui.StatusSuccess, time.Now(), 0, sp)
		expectedLabel := theme.Success("Thought for 0s")
		assert.Contains(t, got, expectedLabel, "Success label should be Success color")
	})

	t.Run("StatusError", func(t *testing.T) {
		got := r.RenderThinking(ui.StatusError, time.Now(), 0, sp)
		expectedLabel := theme.Error("Thought for 0s")
		assert.Contains(t, got, expectedLabel, "Error label should be Error color and duration-based")
	})
}
