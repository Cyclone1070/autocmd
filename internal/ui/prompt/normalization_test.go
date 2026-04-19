package prompt

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/stretchr/testify/assert"
)

func TestNormalizeBlock(t *testing.T) {
	// Simple paragraph
	assert.Equal(t, "\nPara 1", ui.NormalizeBlock("Para 1"))
	
	// Double leading newlines (typical glamour delta)
	assert.Equal(t, "\nPara 2", ui.NormalizeBlock("\n\nPara 2"))
	
	// Mixed ANSI and newlines
	gapLine := "\x1b[0m  \x1b[0m"
	assert.Equal(t, "\n## Header", ui.NormalizeBlock(gapLine+"\n\n## Header"))
	
	// Trailing newlines should be trimmed
	assert.Equal(t, "\nPara 3", ui.NormalizeBlock("Para 3\n\n"))
}

func TestNormalization_ANSI_GapLines(t *testing.T) {
	// Verify that trimVisuallyEmpty handles ANSI gap lines correctly.
	// These are common in glamour header output.
	gapLine := "\x1b[0m  \x1b[0m"
	input := gapLine + "\n" + "## HEADER"
	
	normalized := ui.NormalizeBlock(input)
	assert.Equal(t, "\n## HEADER", normalized, "Should trim ANSI gap line and prepend exactly one newline")
}
