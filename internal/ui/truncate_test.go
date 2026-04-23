package ui

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncatingGater_NoTruncation(t *testing.T) {
	g := NewTruncatingGater(24)
	lines := []string{"Line 1", "Line 2", "Line 3"}
	result, indicatorLines := g.Gate(lines)
	assert.Equal(t, lines, result)
	assert.Equal(t, 0, indicatorLines)
}

func TestTruncatingGater_ShowsIndicatorAndTail(t *testing.T) {
	g := NewTruncatingGater(24)
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("Line %d", i)
	}

	result, indicatorLines := g.Gate(lines)
	assert.Equal(t, 2, indicatorLines)
	assert.Equal(t, "", result[0])
	assert.Equal(t, "  ▲ [8 lines truncated]", result[1])
	assert.Equal(t, "Line 29", result[len(result)-1])
	assert.NotContains(t, result, "Line 0")
}

func TestTruncatingGater_EdgeCases(t *testing.T) {
	lines := []string{"A", "B", "C"}
	g1 := NewTruncatingGater(1)
	g2 := NewTruncatingGater(2)

	res1, indicatorLines1 := g1.Gate(lines)
	assert.Equal(t, []string{"  ▲ [3 lines truncated]"}, res1)
	assert.Equal(t, 1, indicatorLines1)

	res2, indicatorLines2 := g2.Gate(lines)
	assert.Equal(t, []string{"", "  ▲ [3 lines truncated]"}, res2)
	assert.Equal(t, 2, indicatorLines2)
}
