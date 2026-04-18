package prompt

import (
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
)

func TestView_StrictToolboxSpacing(t *testing.T) {
	theme := ui.NewTheme(ui.ThemeConfig{})
	renderer := ui.NewGlamourRenderer(80, true)
	stream := NewStream(renderer)
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	spinnerProvider := ui.NewSpinnerRenderer(lipgloss.NewStyle())

	m := NewModel(nil, nil, tr, spinnerProvider, theme, stream, nil, ui.NewNoOpGater(), 80)

	// Add two tools to simulate the "String 1", "String 2" sequence in the user's image
	m.handleBusEvent(domain.ToolStartEvent{
		CallID:  "1",
		Display: domain.NewBashDisplay("Tool 1", "ls", ""),
	})
	m.handleBusEvent(domain.ToolStartEvent{
		CallID:  "2",
		Display: domain.NewBashDisplay("Tool 2", "pwd", ""),
	})

	view := m.View()

	// Between two boxes, we expect:
	// ╰──────╯ (End of Box 1)
	// \n       (First newline: ends the line)
	// \n       (Second newline: creates the blank line)
	// ╭──────╮ (Start of Box 2)
	//
	// If there is a third \n, it means we have TWO blank lines.
	
	// Print the substring between the boxes
	sep := "╰"
	parts := strings.Split(view, sep)
	if len(parts) > 1 {
		// Get everything between the end of the first box line and the start of the next box
		// The next box starts with \n╭
		afterBox1 := parts[1]
		endOfLine := strings.Index(afterBox1, "\n")
		if endOfLine != -1 {
			gap := afterBox1[endOfLine:]
			nextBoxStart := strings.Index(gap, "╭")
			if nextBoxStart != -1 {
				actualGap := gap[:nextBoxStart]
				t.Logf("Actual gap between boxes: %q", actualGap)
			}
		}
	}

	assert.False(t, strings.Contains(view, "\n\n\n"), "View should not contain triple newlines (double blank lines) between boxes")
}
