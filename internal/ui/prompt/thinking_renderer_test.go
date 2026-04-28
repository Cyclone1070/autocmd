package prompt

import (
	"strings"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
)

type mockSpinner struct{}

func (m *mockSpinner) Frame(_ int) string { return "*" }

type passThroughGater struct{}

func (g *passThroughGater) Gate(lines []string) ([]string, int) { return lines, 0 }

type truncatingMockGater struct {
	called bool
}

func (g *truncatingMockGater) Gate(_ []string) ([]string, int) {
	g.called = true
	return []string{"  ▲ [2 lines truncated]", "kept line"}, 1
}

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
	r := NewThinkingRenderer(theme, 80, &passThroughGater{})
	sp := &mockSpinner{}

	t.Run("StatusRunning", func(t *testing.T) {
		got := r.RenderThinking(ui.StatusRunning, time.Now(), 0, "This is some thinking text\n", sp)
		assert.NotContains(t, got, "...", "Ellipsis should be removed")
		assert.Contains(t, got, "Thinking for")
		assert.Contains(t, got, "⎿ This is some thinking text")
		assert.Contains(t, got, theme.Muted("This is some thinking text"), "Thought content should be fully muted")

		expectedLabel := theme.Primary("Thinking for 0s")
		assert.Contains(t, got, expectedLabel, "Running label should be Primary color")
		assert.Contains(t, got, "\n\n    ", "Thinking block should keep tool-block spacing")
	})

	t.Run("StatusSuccess", func(t *testing.T) {
		got := r.RenderThinking(ui.StatusSuccess, time.Now(), 0, "hidden content", sp)
		expectedLabel := theme.Success("Thought for 0s")
		assert.Contains(t, got, expectedLabel, "Success label should be Success color")
		assert.NotContains(t, got, "⎿", "Completed thought should not show content block")
	})

	t.Run("StatusError", func(t *testing.T) {
		got := r.RenderThinking(ui.StatusError, time.Now(), 0, "hidden content", sp)
		expectedLabel := theme.Error("Thought for 0s")
		assert.Contains(t, got, expectedLabel, "Error label should be Error color and duration-based")
		assert.NotContains(t, got, "⎿", "Completed thought should not show content block")
	})

	t.Run("RunningThoughtIsTruncatedByToolGater", func(t *testing.T) {
		veryLong := strings.Repeat("this thought line keeps growing and should be truncated by tool output gater ", 20) + "\n"
		withRealGater := NewThinkingRenderer(theme, 80, ui.NewToolOutputGater(5))
		got := withRealGater.RenderThinking(ui.StatusRunning, time.Now(), 0, veryLong, sp)
		assert.Contains(t, got, "lines truncated", "long thought content should show truncation indicator from ToolOutputGater")
	})

	t.Run("RunningThoughtWrapsToWidth", func(t *testing.T) {
		narrow := NewThinkingRenderer(theme, 42, &passThroughGater{})
		got := narrow.RenderThinking(ui.StatusRunning, time.Now(), 0, "this line is intentionally very long so we can verify wrapping to avoid content overflow in thinking tool block content", sp)
		for line := range strings.SplitSeq(got, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			assert.LessOrEqual(t, lipgloss.Width(line), 42, "rendered line should not overflow configured width")
		}
	})

	t.Run("InjectedGaterIsUsed", func(t *testing.T) {
		g := &truncatingMockGater{}
		renderer := NewThinkingRenderer(theme, 80, g)
		got := renderer.RenderThinking(ui.StatusRunning, time.Now(), 0, strings.Repeat("text ", 40)+"\n", sp)
		assert.True(t, g.called, "thinking renderer should call injected gater")
		assert.Contains(t, got, "lines truncated")
	})

	t.Run("RunningThoughtRendersOnlyCompletedLines", func(t *testing.T) {
		gotPartial := r.RenderThinking(ui.StatusRunning, time.Now(), 0, "partial only", sp)
		assert.NotContains(t, gotPartial, "⎿", "partial thought without newline should not render content")

		gotMixed := r.RenderThinking(ui.StatusRunning, time.Now(), 0, "line one done\nline two partial", sp)
		assert.Contains(t, gotMixed, "⎿ line one done", "completed line should render")
		assert.NotContains(t, gotMixed, "line two partial", "incomplete tail line should not render")
	})

	t.Run("RunningThoughtRendersCompletedVisualLinesBeforeNewline", func(t *testing.T) {
		narrow := NewThinkingRenderer(theme, 20, &passThroughGater{})
		got := narrow.RenderThinking(ui.StatusRunning, time.Now(), 0, "ABCDEFGHIJKLMNO", sp)
		assert.Contains(t, got, "⎿ ABCDEFGHIJK", "first wrapped visual line should render even without newline")
		assert.NotContains(t, got, "LMNO", "last incomplete visual line should not render yet")
	})
}
