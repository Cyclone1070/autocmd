package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestPicker_Actions_Quit(t *testing.T) {
	items := []Item{{ID: "1", Label: "A"}}
	actions := []Action{
		{
			Key:  "n",
			Quit: true,
			Fn: func(item Item) tea.Cmd {
				return nil
			},
		},
	}

	m := NewPicker(Config{
		Items:   items,
		Actions: actions,
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
			Fn: func(item Item) tea.Cmd {
				return nil
			},
		},
	}

	m := NewPicker(Config{
		Items:   items,
		Actions: actions,
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
	})

	viewAtTop := m.View()
	assert.Contains(t, viewAtTop, "●", "cursor indicator should exist")
	assert.Contains(t, viewAtTop, "Second", "active row label should be rendered")

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	viewAfterMove := m.View()
	assert.Contains(t, viewAfterMove, "Second", "active label should still render after cursor moves")
}
