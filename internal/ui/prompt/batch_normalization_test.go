package prompt

import (
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
)

func TestSpacing_ToolboxBatchNormalization(t *testing.T) {
	theme := ui.NewTheme(ui.ThemeConfig{})
	renderer := ui.NewGlamourRenderer(80, true)
	stream := NewStream(renderer)
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	spinnerProvider := ui.NewSpinnerRenderer(lipgloss.NewStyle())

	m := NewModel(nil, nil, tr, spinnerProvider, theme, stream, ui.NewNoOpGater(), 80)

	// Add two tools
	m.handleBusEvent(domain.ToolStartEvent{
		CallID:  "1",
		Display: domain.NewBashDisplay("Tool 1", "ls", "", ""),
	})
	m.handleBusEvent(domain.ToolStartEvent{
		CallID:  "2",
		Display: domain.NewBashDisplay("Tool 2", "pwd", "", ""),
	})

	// Manually simulate flush batching (no extra spacing inserted by normalizer)
	tools := m.renderAllTools()
	joined := strings.Join(tools, "")
	normalized := ui.NormalizeBlock(joined)

	assert.Contains(t, normalized, "⠹ Tool 1")
	assert.Contains(t, normalized, "⠹ Tool 2")
	assert.NotContains(t, normalized, "╭")
}
