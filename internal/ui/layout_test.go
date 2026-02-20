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
	assert.Contains(t, result, "temporarily truncated")
	assert.Contains(t, result, "Line 29")
	assert.NotContains(t, result, "Line 0")
}
