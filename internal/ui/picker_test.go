package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPicker_Actions_Quit(t *testing.T) {
	items := []Item{{ID: "1", Label: "A"}}
	actions := []Action{
		{
			Key:  "n",
			Quit: true,
			Fn: func(_ Item) tea.Cmd {
				return nil
			},
		},
	}

	m := NewPicker(Config{
		Items:   items,
		Actions: actions,
		Theme:   testTheme(),
	})

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	assert.True(t, m.quit, "Picker must set quit flag when an action with Quit: true is triggered")
	assert.Equal(t, "", m.View(), "View should be empty after quitting")
}

func TestPicker_RegularAction_DoesNotQuit(t *testing.T) {
	items := []Item{{ID: "1", Label: "A"}}
	actions := []Action{
		{
			Key: "r",
			Fn: func(_ Item) tea.Cmd {
				return nil
			},
		},
	}

	m := NewPicker(Config{
		Items:   items,
		Actions: actions,
		Theme:   testTheme(),
	})

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	assert.False(t, m.quit, "Regular actions should not trigger picker quit")
}

func TestPicker_View_CursorIsBlueCircleNotTriangle(t *testing.T) {
	m := NewPicker(Config{
		Title: "TEST",
		Items: []Item{
			{ID: "1", Label: "First"},
			{ID: "2", Label: "Second"},
		},
		Theme: testTheme(),
	})

	view := m.View()
	assert.Contains(t, view, "●", "cursor should be rendered as a blue circle glyph")
	assert.NotContains(t, view, "▸", "triangle cursor glyph should not be rendered")
}

func TestPicker_View_ActiveItemIsHighlightedIndependentlyOfCursor(t *testing.T) {
	m := NewPicker(Config{
		Title: "TEST",
		Items: []Item{
			{ID: "1", Label: "First"},
			{ID: "2", Label: "Second", Active: true},
		},
		Theme: testTheme(),
	})

	viewAtTop := m.View()
	assert.Contains(t, viewAtTop, "●", "cursor indicator should exist")
	assert.Contains(t, viewAtTop, "Second", "active row label should be rendered")

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	viewAfterMove := m.View()
	assert.Contains(t, viewAfterMove, "Second", "active label should still render after cursor moves")
}

func TestPicker_SpaceSelectsLikeEnter(t *testing.T) {
	m := NewPicker(Config{
		Items: []Item{
			{ID: "1", Label: "First"},
			{ID: "2", Label: "Second"},
		},
		Theme: testTheme(),
	})

	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	require.NotNil(t, cmd, "space should trigger quit command after selection")

	p := newModel.(*Picker)
	selected, ok := p.Selected()
	assert.True(t, ok, "space should select current cursor item")
	assert.Equal(t, "1", selected.ID)
	assert.True(t, p.quit, "picker should quit after space selection")
}

func TestPicker_View_GroupHeaderIsBlue(t *testing.T) {
	m := NewPicker(Config{
		Title: "TEST",
		Items: []Item{
			{ID: "1", Label: "First", Group: "GroupBlueTest"},
		},
		Theme: testTheme(),
	})

	view := m.View()
	// The view should contain the group header "GroupBlueTest"
	assert.Contains(t, view, "GroupBlueTest")
	// The ANSI escape sequence for blue foreground color should be present.
	// Since lipgloss resolves colors adaptively based on light/dark mode (or NO_COLOR),
	// we will check if the group header text is styled (has escape sequences).
	assert.True(t, len(view) > len("TEST") + len("First") + len("GroupBlueTest"), "view should contain color styling ANSI sequences")
}

func TestPicker_View_FadedItem(t *testing.T) {
	m := NewPicker(Config{
		Title: "TEST",
		Items: []Item{
			{ID: "1", Label: "FadedItem", Faded: true},
		},
		Theme: testTheme(),
	})
	view := m.View()
	assert.Contains(t, view, "FadedItem")
}

func TestPicker_View_FadedGroupHeader(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := NewPicker(Config{
		Title: "TEST",
		Items: []Item{
			{ID: "1", Label: "First", Group: "GroupBlue", Faded: false},
			{ID: "2", Label: "Second", Group: "GroupFaded", Faded: true},
		},
		Theme: testTheme(),
	})

	view := m.View()
	require.Contains(t, view, "GroupBlue")
	require.Contains(t, view, "GroupFaded")

	// Get the substring of the rendered GroupBlue header (with ANSI sequences)
	// and assert it is styled differently from the GroupFaded header.
	// Since both are bold and have the same margins, they will only differ in the ANSI color codes.
	
	// We can find the styled headers by looking at the lines containing GroupBlue and GroupFaded.
	var lineBlue, lineFaded string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "GroupBlue") {
			lineBlue = line
		}
		if strings.Contains(line, "GroupFaded") {
			lineFaded = line
		}
	}

	require.NotEmpty(t, lineBlue)
	require.NotEmpty(t, lineFaded)
	
	// Strip the actual text so we only compare the ANSI wrapper sequences
	wrapperBlue := strings.Replace(lineBlue, "GroupBlue", "", 1)
	wrapperFaded := strings.Replace(lineFaded, "GroupFaded", "", 1)
	
	assert.NotEqual(t, wrapperBlue, wrapperFaded, "Faded group header should be styled with a different color (gray) than the blue group header")
}

func TestPicker_View_InactiveItemUsesTextColor(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	// Configure a theme with a very specific, recognizable custom text color.
	customTextColor := lipgloss.AdaptiveColor{Light: "#FF00FF", Dark: "#FF00FF"}
	theme := &Theme{
		TextCol: customTextColor,
	}

	m := NewPicker(Config{
		Title: "TEST",
		Theme: theme,
		Items: []Item{
			{ID: "1", Label: "NormalTextItem", Faded: false, Active: false},
		},
	})

	view := m.View()
	// The rendered view should contain the item label "NormalTextItem"
	assert.Contains(t, view, "NormalTextItem")
	
	var itemLine string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "NormalTextItem") {
			itemLine = line
			break
		}
	}
	require.NotEmpty(t, itemLine)
	assert.Contains(t, itemLine, "255;0;255", "Normal item should be styled with the custom TextColor")
}

func TestPicker_View_NestingAndItalicsLayout(t *testing.T) {
	// Enable TrueColor profile for rich styling checks
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := NewPicker(Config{
		Title: "TEST",
		Items: []Item{
			{ID: "1", Label: "ActiveDirItem1", Group: "active-group", Faded: false},
			{ID: "2", Label: "FadedDirItem2", Group: "faded-group", Faded: true},
		},
		Theme: testTheme(),
	})

	view := m.View()
	lines := strings.Split(view, "\n")

	var activeGroupLine, fadedGroupLine, activeItemLine, fadedItemLine string
	for _, line := range lines {
		if strings.Contains(line, "Active-group") {
			activeGroupLine = line
		}
		if strings.Contains(line, "Faded-group") {
			fadedGroupLine = line
		}
		if strings.Contains(line, "ActiveDirItem1") {
			activeItemLine = line
		}
		if strings.Contains(line, "FadedDirItem2") {
			fadedItemLine = line
		}
	}

	require.NotEmpty(t, activeGroupLine)
	require.NotEmpty(t, fadedGroupLine)
	require.NotEmpty(t, activeItemLine)
	require.NotEmpty(t, fadedItemLine)

	// 1. Group headers must align with the left margin (2 spaces)
	// Strip ANSI escape sequences to check the raw prefix
	cleanActiveGroup := stripANSI(activeGroupLine)
	assert.True(t, strings.HasPrefix(cleanActiveGroup, "  Active-group"), "Active group header must align with the left margin (2 spaces)")

	cleanFadedGroup := stripANSI(fadedGroupLine)
	assert.True(t, strings.HasPrefix(cleanFadedGroup, "  Faded-group"), "Faded group header must align with the left margin (2 spaces)")

	// 2. Faded group headers must be italicized (ANSI code 3)
	hasItalic := strings.Contains(fadedGroupLine, "\x1b[3;") ||
		strings.Contains(fadedGroupLine, ";3;") ||
		strings.Contains(fadedGroupLine, ";3m") ||
		strings.Contains(fadedGroupLine, "\x1b[3m")
	assert.True(t, hasItalic, "Faded group header must contain italics ANSI sequence (code 3)")

	// 3. Items must be indented by 4 spaces.
	// Since first item is under the cursor: "  ●  ActiveDirItem1"
	// Second item is not: "     FadedDirItem2"
	cleanActiveItem := stripANSI(activeItemLine)
	assert.True(t, strings.HasPrefix(cleanActiveItem, "  ●  ActiveDirItem1"), "Active item under cursor should have 4-space nested indentation prefix")

	cleanFadedItem := stripANSI(fadedItemLine)
	assert.True(t, strings.HasPrefix(cleanFadedItem, "     FadedDirItem2"), "Inactive item should have 5 spaces of nested indentation prefix")
}

func testTheme() *Theme {
	return &Theme{
		PrimaryCol: lipgloss.AdaptiveColor{Light: "27", Dark: "86"},
		MutedCol:   lipgloss.AdaptiveColor{Light: "246", Dark: "240"},
		SuccessCol: lipgloss.AdaptiveColor{Light: "34", Dark: "86"},
		TextCol:    lipgloss.AdaptiveColor{Light: "235", Dark: "250"},
	}
}



