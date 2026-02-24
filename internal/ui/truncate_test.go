package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncateWithIndicator_NoTruncation(t *testing.T) {
	content := "Line 1\nLine 2\nLine 3"
	result := TruncateWithIndicator(content, 24)
	assert.Equal(t, content, result)
}

func TestTruncateWithIndicator_ShowsIndicatorAndTail(t *testing.T) {
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("Line %d", i)
	}
	content := strings.Join(lines, "\n")

	result := TruncateWithIndicator(content, 24)
	assert.Contains(t, result, "\n  ▲ [8 lines truncated]")
	assert.Contains(t, result, "Line 29")
	assert.NotContains(t, result, "Line 0")
}

func TestTruncateWithIndicator_EdgeCases(t *testing.T) {
	content := "A\nB\nC"

	res1 := TruncateWithIndicator(content, 1)
	assert.Equal(t, "  ▲ [3 lines truncated]", res1)

	res2 := TruncateWithIndicator(content, 2)
	assert.Equal(t, "\n  ▲ [3 lines truncated]", res2)
}
