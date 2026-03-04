package history

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestModel_WidthCapping(t *testing.T) {
	cfg := config.UIConfig{
		ChatWindowWidth: 80,
	}
	messages := []domain.Message{}

	// Case 1: Terminal is wider than config -> should cap to config
	m := NewModel(messages, cfg, 200, 40)
	assert.Equal(t, 80, m.width)

	// Case 2: Terminal is narrower than config -> should cap to terminal
	m = NewModel(messages, cfg, 40, 40)
	assert.Equal(t, 40, m.width)

	// Case 3: Resize to larger than config -> should stay capped at config
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	m = tm.(*Model)
	assert.Equal(t, 80, m.width)
	assert.Equal(t, 50, m.height)

	// Case 4: Resize to smaller than config -> should follow terminal
	tm, _ = m.Update(tea.WindowSizeMsg{Width: 30, Height: 50})
	m = tm.(*Model)
	assert.Equal(t, 30, m.width)
}
