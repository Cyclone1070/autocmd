package history

import (
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
)

func newTestTheme() *ui.Theme {
	return ui.NewTheme(config.DefaultConfig().UI)
}

func TestShellHistory_UseCapturedOutput(t *testing.T) {
	theme := newTestTheme()
	captured := "output line 1\noutput line 2"

	messages := []domain.Message{
		{
			Role: domain.RoleAssistant,
			ToolCalls: []domain.ToolCall{
				{ID: "tc-1", Name: "shell"},
			},
			ToolDisplays: map[string]domain.ToolDisplay{
				"tc-1": domain.ShellDisplay{
					TypeField:      "shell",
					Command:        "ls",
					CapturedOutput: &captured,
				},
			},
		},
		{
			Role:       domain.RoleTool,
			ToolCallID: "tc-1",
			Content:    "output line 1\noutput line 2\n\n(Exit code: 0)",
		},
	}

	rendered := BuildHistory(messages, nil, theme, 80)

	// Should contain the captured output
	assert.Contains(t, rendered, "output line 1")
	assert.Contains(t, rendered, "output line 2")
	// Should NOT contain the exit code decoration from tool response
	assert.NotContains(t, rendered, "(Exit code: 0)")
}

func TestShellHistory_EmptyStdout_NoExitCode(t *testing.T) {
	theme := newTestTheme()
	empty := ""

	messages := []domain.Message{
		{
			Role: domain.RoleAssistant,
			ToolCalls: []domain.ToolCall{
				{ID: "tc-1", Name: "shell"},
			},
			ToolDisplays: map[string]domain.ToolDisplay{
				"tc-1": domain.ShellDisplay{
					TypeField:      "shell",
					Command:        "touch t.txt",
					CapturedOutput: &empty,
				},
			},
		},
		{
			Role:       domain.RoleTool,
			ToolCallID: "tc-1",
			Content:    "\n\n(Exit code: 0)",
		},
	}

	rendered := BuildHistory(messages, nil, theme, 80)

	// Should NOT contain the exit code decoration
	// THIS TEST IS EXPECTED TO FAIL IN THE RED PHASE
	assert.NotContains(t, rendered, "(Exit code: 0)", "History should not leak exit code for successful empty stdout commands")
}

func TestShellHistory_NilCapturedOutput_Fallback(t *testing.T) {
	theme := newTestTheme()

	messages := []domain.Message{
		{
			Role: domain.RoleAssistant,
			ToolCalls: []domain.ToolCall{
				{ID: "tc-1", Name: "shell"},
			},
			ToolDisplays: map[string]domain.ToolDisplay{
				"tc-1": domain.ShellDisplay{
					TypeField:      "shell",
					Command:        "ls",
					CapturedOutput: nil, // Legacy session
				},
			},
		},
		{
			Role:       domain.RoleTool,
			ToolCallID: "tc-1",
			Content:    "fallback output\n\n(Exit code: 0)",
		},
	}

	rendered := BuildHistory(messages, nil, theme, 80)

	// Should fall back to tool response content
	assert.Contains(t, rendered, "fallback output")
	assert.Contains(t, rendered, "(Exit code: 0)")
}

func TestShellHistory_ErrorStatus(t *testing.T) {
	theme := newTestTheme()
	empty := ""

	messages := []domain.Message{
		{
			Role: domain.RoleAssistant,
			ToolCalls: []domain.ToolCall{
				{ID: "tc-1", Name: "shell"},
			},
			ToolDisplays: map[string]domain.ToolDisplay{
				"tc-1": domain.ShellDisplay{
					TypeField:      "shell",
					Command:        "false",
					CapturedOutput: &empty,
				},
			},
		},
		{
			Role:       domain.RoleTool,
			ToolCallID: "tc-1",
			Content:    "Error: Execution failed",
			ToolError:  true,
		},
	}

	rendered := BuildHistory(messages, nil, theme, 80)

	// Should contain the error prefix indicator (X or similar depending on theme)
	assert.Contains(t, rendered, "✗")
}

func TestDivider_Color(t *testing.T) {
	// Force color profile for consistent testing of escape codes
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii) // Reset after test

	cfg := config.DefaultConfig().UI
	theme := ui.NewTheme(cfg)

	// Create expected USER divider (primary color)
	primaryStyle := lipgloss.NewStyle().Foreground(theme.PrimaryColor())
	expectedUserDivider := primaryStyle.Render(strings.Repeat("-", 80))

	// Create expected ASSISTANT divider (muted color)
	mutedStyle := lipgloss.NewStyle().Foreground(theme.MutedColor())
	expectedAssistantDivider := mutedStyle.Render(strings.Repeat("-", 80))

	messages := []domain.Message{
		{Role: domain.RoleUser, Content: "user content"},
		{Role: domain.RoleAssistant, Content: "assistant content"},
	}

	// Render USER message
	renderedUser := RenderMessage(messages, 0, nil, theme, 80, false)
	assert.Contains(t, renderedUser, expectedUserDivider, "USER divider should use primary color")

	// Render ASSISTANT message
	renderedAssistant := RenderMessage(messages, 1, nil, theme, 80, true)
	assert.Contains(t, renderedAssistant, expectedAssistantDivider, "ASSISTANT divider should use muted color")
}
