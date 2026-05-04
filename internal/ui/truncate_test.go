package ui

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncatingGater_NoTruncation(t *testing.T) {
	g := NewTruncatingGater(24)
	lines := []string{"Line 1", "Line 2", "Line 3"}
	theme := NewTheme(ThemeConfig{})
	result, _ := g.Gate(lines, 0, false, theme)
	assert.Equal(t, lines, result)
}

func TestTruncatingGater_ShowsIndicatorAndTail(t *testing.T) {
	g := NewTruncatingGater(24)
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("Line %d", i)
	}

	theme := NewTheme(ThemeConfig{})
	result, _ := g.Gate(lines, 0, true, theme)
	// Key is styled with Primary, labels with Muted.
	expected := theme.Muted("    ▲ [7 lines above]") + "  " + theme.Primary("Ctrl+u") + theme.Muted(" scroll up")
	assert.Equal(t, expected, result[0])
	assert.Equal(t, "Line 29", result[len(result)-1])
	assert.NotContains(t, result, "Line 0")
}

func TestTruncatingGater_EdgeCases(t *testing.T) {
	lines := []string{"A", "B", "C"}
	g1 := NewTruncatingGater(1)
	g2 := NewTruncatingGater(2)
	theme := NewTheme(ThemeConfig{})

	res1, _ := g1.Gate(lines, 0, false, theme)
	// fallback to simple truncation because budget <= 2*H
	assert.Equal(t, []string{"C"}, res1)

	res2, _ := g2.Gate(lines, 0, false, theme)
	// fallback to simple truncation because budget <= 2*H
	assert.Equal(t, []string{"B", "C"}, res2)
}

func TestTruncatingGater_NoScrollInstructionsWhenNotScrollable(t *testing.T) {
	g := NewTruncatingGater(24)
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("Line %d", i)
	}

	theme := NewTheme(ThemeConfig{})
	result, _ := g.Gate(lines, 0, false, theme)
	assert.Equal(t, theme.Muted("    ▲ [7 lines above]"), result[0])
	assert.NotContains(t, result[0], "scroll")
}
