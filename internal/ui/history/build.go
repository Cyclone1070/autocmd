package history

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/charmbracelet/lipgloss"
)

// BuildHistory constructs a pre-rendered string representation of the session history.
func BuildHistory(messages []domain.Message, renderer ui.Renderer, theme *ui.Theme, width int) string {
	var sb strings.Builder

	for i := range messages {
		sb.WriteString(RenderMessage(messages, i, renderer, theme, width, i > 0))
	}

	return sb.String()
}

// RenderMessage renders a single message at the given index.
// If includeLeadingNewline is true, it prepends a newline before the divider.
func RenderMessage(messages []domain.Message, idx int, renderer ui.Renderer, theme *ui.Theme, width int, includeLeadingNewline bool) string {
	var sb strings.Builder
	msg := messages[idx]

	switch msg.Role {
	case domain.RoleUser:
		renderUserMessage(&sb, msg, idx, renderer, theme, width, includeLeadingNewline)
	case domain.RoleAssistant:
		renderAssistantMessage(&sb, messages, idx, renderer, theme, width, includeLeadingNewline)
	}

	return sb.String()
}

func renderDivider(sb *strings.Builder, theme *ui.Theme, width int, includeLeadingNewline bool) {
	line := strings.Repeat("-", width)
	style := lipgloss.NewStyle().Foreground(theme.MutedColor())
	prefix := ""
	if includeLeadingNewline {
		prefix = "\n"
	}
	fmt.Fprintf(sb, "%s%s\n", prefix, style.Render(line))
}

func renderUserMessage(sb *strings.Builder, msg domain.Message, idx int, renderer ui.Renderer, theme *ui.Theme, width int, includeLeadingNewline bool) {
	renderDivider(sb, theme, width, includeLeadingNewline)

	style := lipgloss.NewStyle().
		Foreground(theme.PrimaryColor()).
		Bold(true)

	// Divider already handles vertical spacing
	fmt.Fprintf(sb, "%s\n", style.Render("YOU:"))

	content := msg.Content
	if renderer != nil {
		content = renderer.Render(msg.Content)
	}
	fmt.Fprintf(sb, "%s", content)
}

func renderAssistantMessage(sb *strings.Builder, messages []domain.Message, idx int, renderer ui.Renderer, theme *ui.Theme, width int, includeLeadingNewline bool) {
	msg := messages[idx]

	printHeader := true
	if idx > 0 {
		prevRole := messages[idx-1].Role
		if prevRole == domain.RoleAssistant || prevRole == domain.RoleTool {
			printHeader = false
		}
	}

	if printHeader {
		renderDivider(sb, theme, width, includeLeadingNewline)

		style := lipgloss.NewStyle().
			Foreground(theme.MutedColor()).
			Bold(true)

		// Divider already handles vertical spacing
		fmt.Fprintf(sb, "%s\n", style.Render("ASSISTANT:"))
	}

	if msg.Content != "" {
		content := msg.Content
		if renderer != nil {
			content = renderer.Render(msg.Content)
		}
		fmt.Fprintf(sb, "%s", content)
	}

	// Always render tool calls if present, using their baked displays
	for _, tc := range msg.ToolCalls {
		display, ok := msg.ToolDisplays[tc.ID]
		if !ok {
			continue
		}

		// Status determination: look for the tool result message
		status := ui.StatusSuccess
		var toolOutput string
		var toolErr string

		for j := idx + 1; j < len(messages); j++ {
			msgJ := messages[j]
			if msgJ.Role == domain.RoleTool && msgJ.ToolCallID == tc.ID {
				toolOutput = msgJ.Content
				if msgJ.ToolError {
					status = ui.StatusError
					toolErr = strings.TrimPrefix(toolOutput, "Error:")
					toolErr = strings.TrimSpace(toolErr)
				}
				break
			}
		}

		var rendered string
		prefix := "✓"
		if status == ui.StatusError {
			prefix = "✗"
		}

		// Use width-2 for the content to account for the box borders,
		// matching the logic in engine.go
		boxWidth := width - 2

		switch d := display.(type) {
		case domain.StringDisplay:
			rendered = ui.RenderString(theme, d, status, toolErr, prefix)
		case domain.DiffDisplay:
			rendered = ui.RenderDiff(boxWidth, theme, d, status, toolErr, prefix)
		case domain.ShellDisplay:
			// Prefer baked captured output over the decorated toolOutput (which includes exit codes for LLM)
			output := toolOutput
			if d.CapturedOutput != nil {
				output = *d.CapturedOutput
			}
			rendered = ui.RenderShell(boxWidth, 10, theme, d, output, status, toolErr, prefix)
		}

		if rendered != "" {
			// Boxes already have their own internal vertical padding.
			// We only want a single newline to separate them from text or other boxes.
			boxed := strings.TrimLeft(theme.Box(rendered, boxWidth, status), "\n")
			fmt.Fprint(sb, boxed)
		}
	}
}
