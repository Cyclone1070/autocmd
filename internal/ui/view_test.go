package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/stretchr/testify/assert"
)

// Test helpers (inlined)
// Note: mockCursorDetector is defined in model_test.go since both files share the same package

func newTestModelForView(t *testing.T) *model {
	t.Helper()
	cfg := config.DefaultConfig()
	cd := mockCursorDetector{row: 1}
	m, err := newModel(cfg, cd)
	if err != nil {
		t.Fatalf("failed to create model: %v", err)
	}
	m.width = 80
	m.termHeight = 24
	return m
}

// Truncation tests - property-level, not exact cutoffs

func TestTruncateWithIndicator_NoTruncation(t *testing.T) {
	content := "Line 1\nLine 2\nLine 3"
	result := truncateWithIndicator(content, 24)
	assert.Equal(t, content, result)
}

func TestTruncateWithIndicator_ShowsIndicatorAndTail(t *testing.T) {
	// Create content with more lines than maxLines
	// maxLines = max(24-5, 5) = 19, so we need >19 lines
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("Line %d", i)
	}
	content := strings.Join(lines, "\n")

	result := truncateWithIndicator(content, 24)
	// Property: should have truncation indicator when content exceeds limit
	assert.Contains(t, result, "temporarily truncated")
	// Property: should show tail (last lines)
	// Don't assert exact line numbers - just that tail is present and early lines are not
	assert.Contains(t, result, "Line 29") // Last line should be present
	assert.NotContains(t, result, "Line 0") // Early lines should be truncated
}

// Status bar format tests - lightweight unit tests for format-level behavior
// Layout tests (wide/narrow) are covered by integration tests

func TestStatusBar_Done(t *testing.T) {
	m := newTestModelForView(t)
	m.width = 80
	m.runState = stateDone

	status := m.statusBar()
	assert.Contains(t, status, "✓")
	assert.Contains(t, status, "Done")
	assert.Contains(t, status, "Context: 42%")
}

func TestStatusBar_Cancelled(t *testing.T) {
	m := newTestModelForView(t)
	m.width = 80
	m.runState = stateCancelled

	status := m.statusBar()
	assert.Contains(t, status, "✗")
	assert.Contains(t, status, "Cancelled")
	assert.Contains(t, status, "Context: 42%")
}
