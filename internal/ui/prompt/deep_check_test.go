package prompt

import (
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/charmbracelet/lipgloss"
)

func TestDeepCheck(t *testing.T) {
	theme := ui.NewTheme(ui.ThemeConfig{})
	renderer := ui.NewGlamourRenderer(80, true)
	stream := NewStream(renderer)
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	sp := ui.NewSpinnerRenderer(lipgloss.NewStyle())

	m := NewModel(nil, nil, tr, sp, theme, stream, ui.NewNoOpGater(), 80)

	// Add tools
	m.handleBusEvent(domain.ToolStartEvent{CallID: "1", Display: domain.NewBashDisplay("ls", "ls", "", "")})
	m.handleBusEvent(domain.ToolEndEvent{CallID: "1", Display: domain.NewBashDisplay("ls", "ls", "", "")})
	m.handleBusEvent(domain.ToolStartEvent{CallID: "2", Display: domain.NewBashDisplay("pwd", "pwd", "", "")})

	tools := m.renderAllTools()
	for i, tool := range tools {
		t.Logf("Tool %d: %q", i, tool)
	}

	joined := strings.Join(tools, "")
	t.Logf("Joined: %q", joined)
}
