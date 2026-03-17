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
