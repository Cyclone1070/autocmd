package ui

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// stripANSI removes ANSI escape codes from a string.
func stripANSI(str string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*[mGKH]`)
	return re.ReplaceAllString(str, "")
}

func TestRenderer_RedBarSymmetry(t *testing.T) {
	r := NewGlamourRenderer(80, true)
	markdown := "```go\nfunc hello() {}\n```"
	rendered := r.Render(markdown)

	lines := strings.Split(rendered, "\n")

	// Check for the red bar character ┃
	var barIndices []int
	for i, line := range lines {
		if strings.Contains(line, "┃") {
			barIndices = append(barIndices, i)
		}
	}

	// 1. Bar should exist
	assert.NotEmpty(t, barIndices, "Should have a red vertical bar for code blocks")

	// 2. Bar should be symmetric (at least one blank barred line at top and bottom)
	topBlanks := 0
	for i := 0; i < len(barIndices); i++ {
		stripped := stripANSI(lines[barIndices[i]])
		// Content is everything after the bar
		content := strings.TrimSpace(strings.Replace(stripped, "┃", "", 1))
		if content == "" {
			topBlanks++
		} else {
			break
		}
	}

	bottomBlanks := 0
	for i := range slices.Backward(barIndices) {
		stripped := stripANSI(lines[barIndices[i]])
		content := strings.TrimSpace(strings.Replace(stripped, "┃", "", 1))
		if content == "" {
			bottomBlanks++
		} else {
			break
		}
	}

	assert.Equal(t, 0, topBlanks, "Should have no blank barred line at top")
	assert.Equal(t, 0, bottomBlanks, "Should have no blank barred line at bottom")
}

func TestRenderer_BlockquoteAlignment(t *testing.T) {
	r := NewGlamourRenderer(80, true)
	markdown := "> blockquote\n\n```go\ncode\n```"
	rendered := r.Render(markdown)

	lines := strings.Split(rendered, "\n")

	quoteBarPos := -1
	codeBarPos := -1

	for _, line := range lines {
		stripped := stripANSI(line)
		if strings.Contains(stripped, "│") && quoteBarPos == -1 {
			quoteBarPos = strings.Index(stripped, "│")
		}
		if strings.Contains(stripped, "┃") && codeBarPos == -1 {
			codeBarPos = strings.Index(stripped, "┃")
		}
	}

	assert.NotEqual(t, -1, quoteBarPos, "Should find blockquote bar")
	assert.NotEqual(t, -1, codeBarPos, "Should find code block bar")
	assert.Equal(t, quoteBarPos, codeBarPos, "Code bar should align perfectly with blockquote bar")
}

func TestRenderer_CodeBlockTrailingSpacing(t *testing.T) {
	r := NewGlamourRenderer(80, true)
	markdown := "```go\ncode\n```\nFollow up text."
	rendered := r.Render(markdown)

	// We expect a sequence like:
	// ... barred line ...
	// (blank line)
	// Follow up text.

	lines := strings.Split(rendered, "\n")
	lastBarIdx := -1
	for i, line := range lines {
		if strings.Contains(line, "┃") {
			lastBarIdx = i
		}
	}

	assert.NotEqual(t, -1, lastBarIdx, "Should find code block bar")

	// Check lines after the last bar
	remaining := lines[lastBarIdx+1:]

	// Filter out completely empty strings resulting from trailing newlines in Join
	var nonEmtpy []string
	for _, s := range remaining {
		if s != "" {
			nonEmtpy = append(nonEmtpy, s)
		}
	}

	assert.GreaterOrEqual(t, len(nonEmtpy), 2, "Should have a blank line and then text after code block")
	assert.Contains(t, stripANSI(nonEmtpy[len(nonEmtpy)-1]), "Follow up text", "Final line should be follow up text")

	// The line immediately after the last bar should be blank (white space only)
	assert.True(t, strings.TrimSpace(stripANSI(remaining[0])) == "", "Line immediately after code block should be blank")
}

func TestRenderer_DiagramNoHighlight(t *testing.T) {
	r := NewGlamourRenderer(80, true)
	// The large diagram that triggered the red lines in the history
	diagram := "┌─────────────────────────────────────────────────────────────┐\n" +
		"│                          cmd/                               │\n" +
		"│  Wiring layer. Creates concrete instances, connects them    │\n" +
		"│  via dependency injection, and manages lifecycle.           │\n" +
		"└────────┬──────────────────┬──────────────────┬──────────────┘\n" +
		"                 │ injects          │ injects          │ injects\n" +
		"                 ▼                  ▼                  ▼\n" +
		"        ┌──────────────┐      ┌───────────────┐   ┌────────────────────┐\n" +
		"        │  workflow/   │      │    ui/        │   │  internal services │\n" +
		"        │ Orchestrator │◄────►│  Reactive     │   │  agent/  auth/     │\n" +
		"        │ coordinates  │events│  event-driven │   │  config/ fs/       │\n" +
		"        │ use cases    │      │  display      │   │  llm/    session/  │\n" +
		"        │              │      │               │   │  state/  tool/     │\n" +
		"        └──────────────┘      └───────────────┘   └────────────────────┘"

	markdown := "```\n" + diagram + "\n```"
	rendered := r.Render(markdown)

	// Check for the red background ANSI code \x1b[48;5;203m which indicates error highlighting.
	// This should be absent now that we've disabled error backgrounds in the style.
	assert.NotContains(t, rendered, "\x1b[48;5;203m", "Should not contain red background error highlighting for diagrams")
}

func TestRenderer_InlineCodePadding(t *testing.T) {
	r := NewGlamourRenderer(80, true)
	markdown := "use `read_file` tool"
	rendered := r.Render(markdown)
	stripped := stripANSI(rendered)

	// If padding is present, it will have "use  read_file  tool" (double spaces)
	assert.Contains(t, stripped, "use read_file tool", "Inline code should not have padding")
	assert.NotContains(t, stripped, "use  read_file  tool", "Inline code should not have double spaces from padding")
}

func TestRenderer_LaTeXSanitization(t *testing.T) {
	r := NewGlamourRenderer(80, true)
	// Common LLM symbols
	markdown := "Step 1 $\\rightarrow$ Step 2 $\\checkmark$"
	rendered := r.Render(markdown)
	stripped := stripANSI(rendered)

	assert.Contains(t, stripped, "Step 1 → Step 2 ✓")
}
