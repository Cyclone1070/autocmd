package ui

import (
	"testing"

	"github.com/charmbracelet/glamour"
)

// WrapGlamour adapts an existing glamour.TermRenderer to Renderer (for tests).
func WrapGlamour(tr *glamour.TermRenderer) Renderer {
	return &GlamourRenderer{tr: tr}
}

func TestPlaceHolder(t *testing.T) {
	// Simple placeholder test to make the file valid and lint-free if needed
}
