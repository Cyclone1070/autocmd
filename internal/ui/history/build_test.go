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
	cfg := config.DefaultConfig()
	cfg.UI.ShortToolbox = false
	return ui.NewTheme(cfg.UI)
}

func TestShellHistory_UseCapturedOutput(t *testing.T) {
	theme := newTestTheme()
	captured := "output line 1\noutput line 2"

	messages := domain.Messages{
		domain.AssistantMessage{
			ToolCalls: []domain.ToolCall{
				{ID: "tc-1", Name: "shell"},
			},
		},
		domain.ToolMessage{
			ToolCallID: "tc-1",
			Content:    "output line 1\noutput line 2\n\n(Exit code: 0)",
		},
	}

	displays := domain.ToolDisplays{
		"tc-1": domain.ShellDisplay{
			TypeField:      "shell",
			Command:        "ls",
			CapturedOutput: &captured,
		},
	}

	rendered := BuildHistory(&domain.Session{Messages: messages, ToolDisplays: displays}, nil, theme, 80)

	// Should contain the captured output
	assert.Contains(t, rendered, "output line 1")
	assert.Contains(t, rendered, "output line 2")
	// Should NOT contain the exit code decoration from tool response
	assert.NotContains(t, rendered, "(Exit code: 0)")
}

func TestShellHistory_EmptyStdout_NoExitCode(t *testing.T) {
	theme := newTestTheme()
	empty := ""

	messages := domain.Messages{
		domain.AssistantMessage{
			ToolCalls: []domain.ToolCall{
				{ID: "tc-1", Name: "shell"},
			},
		},
		domain.ToolMessage{
			ToolCallID: "tc-1",
			Content:    "\n\n(Exit code: 0)",
		},
	}

	displays := domain.ToolDisplays{
		"tc-1": domain.ShellDisplay{
			TypeField:      "shell",
			Command:        "touch t.txt",
			CapturedOutput: &empty,
		},
	}

	rendered := BuildHistory(&domain.Session{Messages: messages, ToolDisplays: displays}, nil, theme, 80)

	// Should NOT contain the exit code decoration
	// THIS TEST IS EXPECTED TO FAIL IN THE RED PHASE
	assert.NotContains(t, rendered, "(Exit code: 0)", "History should not leak exit code for successful empty stdout commands")
}

func TestShellHistory_NilCapturedOutput_Fallback(t *testing.T) {
	theme := newTestTheme()

	messages := domain.Messages{
		domain.AssistantMessage{
			ToolCalls: []domain.ToolCall{
				{ID: "tc-1", Name: "shell"},
			},
		},
		domain.ToolMessage{
			ToolCallID: "tc-1",
			Content:    "fallback output\n\n(Exit code: 0)",
		},
	}

	displays := domain.ToolDisplays{
		"tc-1": domain.ShellDisplay{
			TypeField:      "shell",
			Command:        "ls",
			CapturedOutput: nil, // Legacy session
		},
	}

	rendered := BuildHistory(&domain.Session{Messages: messages, ToolDisplays: displays}, nil, theme, 80)

	// Should fall back to tool response content
	assert.Contains(t, rendered, "fallback output")
	assert.Contains(t, rendered, "(Exit code: 0)")
}

func TestShellHistory_ErrorStatus(t *testing.T) {
	theme := newTestTheme()
	empty := ""

	messages := domain.Messages{
		domain.AssistantMessage{
			ToolCalls: []domain.ToolCall{
				{ID: "tc-1", Name: "shell"},
			},
		},
		domain.ToolMessage{
			ToolCallID: "tc-1",
			Content:    "Error: Execution failed",
			ToolError:  true,
		},
	}

	displays := domain.ToolDisplays{
		"tc-1": domain.ShellDisplay{
			TypeField:      "shell",
			Command:        "false",
			CapturedOutput: &empty,
		},
	}

	rendered := BuildHistory(&domain.Session{Messages: messages, ToolDisplays: displays}, nil, theme, 80)

	// Should contain the error prefix indicator (X or similar depending on theme)
	assert.Contains(t, rendered, "✘")
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

	messages := domain.Messages{
		domain.UserMessage{Content: "user content"},
		domain.AssistantMessage{Content: "assistant content"},
	}

	// Render USER message
	renderedUser := RenderMessage(messages, 0, nil, nil, theme, 80, false)
	assert.Contains(t, renderedUser, expectedUserDivider, "USER divider should use primary color")

	// Render ASSISTANT message
	renderedAssistant := RenderMessage(messages, 1, nil, nil, theme, 80, true)
	assert.Contains(t, renderedAssistant, expectedAssistantDivider, "ASSISTANT divider should use muted color")
}
func TestIssue_History_ToolBoxLeadingNewline(t *testing.T) {
	// RED PHASE: This test should fail because tool box is currently trimmed.
	theme := newTestTheme()
	width := 80
	renderer := ui.NewGlamourRenderer(width, true)

	tcID := "1"
	msg := domain.AssistantMessage{
		Content: "thought",
		ToolCalls: []domain.ToolCall{
			{
				ID:   tcID,
				Name: "shell",
			},
		},
	}
	messages := domain.Messages{msg}
	displays := domain.ToolDisplays{
		tcID: domain.NewShellDisplay("header", "ls", nil, nil),
	}

	var sb strings.Builder
	renderAssistantMessage(&sb, msg, messages, 0, displays, renderer, theme, width, false)
	rendered := sb.String()

	// Re-rendering a box to see its start
	rawBox := theme.Box("header", width-2, ui.StatusSuccess)
	assert.True(t, strings.HasPrefix(rawBox, "\n"), "Raw theme.Box should start with a newline")

	// We verify that the rendered history contains the box (starting with its unique top border)
	// and that it is preceded by at least one newline (for spacing).
	topBorder := "╭──────────────────────────────────────────────────────────────────────────────╮"
	assert.Contains(t, rendered, topBorder, "Rendered history should contain the tool box top border")

	// Check that the character immediately preceding the top border is a newline.
	borderIdx := strings.Index(rendered, topBorder)
	assert.Greater(t, borderIdx, 0, "Top border should not be at the very start")
	assert.Equal(t, uint8('\n'), rendered[borderIdx-1], "Top border should be preceded by a newline")
}

func TestMessageHeaders(t *testing.T) {
	// Force color profile for consistent testing
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	theme := newTestTheme()
	width := 80

	messages := domain.Messages{
		domain.UserMessage{Content: "hello"},
		domain.AssistantMessage{Content: "hi"},
	}

	t.Run("User Message Formatting", func(t *testing.T) {
		theme := newTestTheme()
		width := 80
		renderer := &mockRenderer{}
		messages := domain.Messages{
			domain.UserMessage{Content: "Hello World"},
		}

		rendered := RenderMessage(messages, 0, nil, renderer, theme, width, false)

		// Assert EXACTLY one blank line (header + one newline from build.go, then one newline from renderer)
		assert.Contains(t, rendered, "USER:", "Should contain USER: header")
		// \x1b\[[0-9;]*m matches any ANSI escape code
		assert.Regexp(t, `USER:(\x1b\[[0-9;]*m)?\n\nHello World\[rendered\]`, rendered)
		assert.NotRegexp(t, `USER:(\x1b\[[0-9;]*m)?\n\n\n`, rendered)
		assert.Contains(t, rendered, "Hello World[rendered]", "User message content should be processed by renderer")
	})

	t.Run("Assistant Message Spacing", func(t *testing.T) {
		theme := newTestTheme()
		width := 80
		tcID := "tc-1"
		messages := domain.Messages{
			domain.AssistantMessage{
				ToolCalls: []domain.ToolCall{{ID: tcID, Name: "shell"}},
			},
		}
		displays := domain.ToolDisplays{
			tcID: domain.NewShellDisplay("header", "ls", nil, nil),
		}

		rendered := RenderMessage(messages, 0, displays, nil, theme, width, false)

		// Assert exactly one blank line (ASSISTANT:\n + Box start \n)
		assert.Contains(t, rendered, "ASSISTANT:", "Should contain ASSISTANT: header")
		assert.Regexp(t, `ASSISTANT:(\x1b\[[0-9;]*m)?\n\n(\x1b\[[0-9;]*m)?╭`, rendered)
		assert.NotRegexp(t, `ASSISTANT:(\x1b\[[0-9;]*m)?\n\n\n`, rendered)
	})

	t.Run("Assistant Header", func(t *testing.T) {
		rendered := RenderMessage(messages, 1, nil, nil, theme, width, false)

		// Should contain "ASSISTANT:" in bold
		style := lipgloss.NewStyle().Foreground(theme.MutedColor()).Bold(true)
		expected := style.Render("ASSISTANT:")
		assert.Contains(t, rendered, expected)
	})

	t.Run("User Header", func(t *testing.T) {
		rendered := RenderMessage(messages, 0, nil, nil, theme, width, false)

		// Should contain "USER:" in bold
		style := lipgloss.NewStyle().Foreground(theme.PrimaryColor()).Bold(true)
		expected := style.Render("USER:")
		assert.Contains(t, rendered, expected)
	})
}

type mockRenderer struct{}

func (m *mockRenderer) Render(s string) string {
	// Real Glamour renderer adds a leading newline
	return "\n" + s + "[rendered]"
}
