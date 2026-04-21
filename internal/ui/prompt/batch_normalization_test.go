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
		Display: domain.NewBashDisplay("Tool 1", "ls", ""),
	})
	m.handleBusEvent(domain.ToolStartEvent{
		CallID:  "2",
		Display: domain.NewBashDisplay("Tool 2", "pwd", ""),
	})

	// Manually simulate what our new joinAndNormalize will do
	tools := m.renderAllTools()
	joined := strings.Join(tools, "")
	normalized := ui.NormalizeBlock(joined)

	// We expect ONE blank line between boxes. 
	// This means Box 1 ends with ╯ and Box 2 starts with \n\n.
	// So we look for ╯\n\n╭
	
	// Currently, Theme.Box only adds ONE \n, so this test should FAIL 
	// (it will only find ╯\n╭ which is zero blank lines)
	assert.Contains(t, normalized, "╯\n\n╭", "There should be exactly one blank line between toolboxes")
}
