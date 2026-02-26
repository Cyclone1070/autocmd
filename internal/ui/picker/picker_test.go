package picker

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestPicker_Navigation(t *testing.T) {
	items := []Item{
		{ID: "1", Label: "A", Group: "Group 1"},
		{ID: "2", Label: "B", Group: "Group 1"},
		{ID: "3", Label: "C", Group: "Group 2"},
	}

	m := NewPicker(Config{
		Title: "TEST",
		Items: items,
	})

	// Initial cursor at first selectable item
	assert.Equal(t, 0, m.cursor)

	// Move down
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	assert.Equal(t, 1, m.cursor)

	// Move down again
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	assert.Equal(t, 2, m.cursor)

	// Select
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	selected, ok := m.Selected()
	assert.True(t, ok)
	assert.Equal(t, "3", selected.ID)
}

func TestPicker_Actions(t *testing.T) {
	items := []Item{
		{ID: "1", Label: "A"},
	}

	actionCalled := false
	actions := []Action{
		{
			Key: "r",
			Fn: func(item Item) tea.Cmd {
				actionCalled = true
				return nil
			},
		},
	}

	m := NewPicker(Config{
		Items:   items,
		Actions: actions,
	})

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	assert.True(t, actionCalled)
}

func TestPicker_RefreshItems(t *testing.T) {
	items := []Item{
		{ID: "1", Label: "A"},
		{ID: "2", Label: "B"},
	}

	m := NewPicker(Config{
		Items: items,
	})

	// Move to second item
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	assert.Equal(t, 1, m.cursor)

	// Refresh with only one item
	m.RefreshItems([]Item{{ID: "1", Label: "A"}})

	// Cursor should be adjusted to last valid index
	assert.Equal(t, 0, m.cursor)
}

func TestPicker_EmptyList(t *testing.T) {
	m := NewPicker(Config{
		Items: []Item{},
	})

	// Navigation should not crash
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	assert.Equal(t, 0, m.cursor)

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	assert.Equal(t, 0, m.cursor)

	// View should not crash
	view := m.View()
	assert.Contains(t, view, "No entries found.")
}
