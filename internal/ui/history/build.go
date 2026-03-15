package history

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/charmbracelet/lipgloss"
)

// BuildHistory constructs a pre-rendered string representation of the session history.
func BuildHistory(session *domain.Session, renderer ui.Renderer, theme *ui.Theme, width int) string {
	var sb strings.Builder
	messages := session.Messages
	displays := session.ToolDisplays

	for i := range messages {
		sb.WriteString(RenderMessage(messages, i, displays, renderer, theme, width, i > 0))
	}

	return sb.String()
}

// RenderMessage renders a single message at the given index.
// If includeLeadingNewline is true, it prepends a newline before the divider.
func RenderMessage(messages domain.Messages, idx int, displays domain.ToolDisplays, renderer ui.Renderer, theme *ui.Theme, width int, includeLeadingNewline bool) string {
	var sb strings.Builder
	msg := messages[idx]

	switch m := msg.(type) {
	case domain.UserMessage:
		renderUserMessage(&sb, m, renderer, theme, width, includeLeadingNewline)
	case domain.AssistantMessage:
		renderAssistantMessage(&sb, m, messages, idx, displays, renderer, theme, width, includeLeadingNewline)
	}

	return sb.String()
}

func renderDivider(sb *strings.Builder, theme *ui.Theme, width int, color lipgloss.AdaptiveColor, includeLeadingNewline bool) {
	line := strings.Repeat("-", width)
	style := lipgloss.NewStyle().Foreground(color)
	prefix := ""
	if includeLeadingNewline {
		prefix = "\n"
	}
	fmt.Fprintf(sb, "%s%s\n", prefix, style.Render(line))
}

func renderUserMessage(sb *strings.Builder, msg domain.UserMessage, renderer ui.Renderer, theme *ui.Theme, width int, includeLeadingNewline bool) {
	renderDivider(sb, theme, width, theme.PrimaryColor(), includeLeadingNewline)

	style := lipgloss.NewStyle().
		Foreground(theme.PrimaryColor()).
		Bold(true)

	// Divider already handles vertical spacing
	fmt.Fprintf(sb, "%s\n", style.Render("USER:"))

	content := msg.Content
	if renderer != nil {
		content = renderer.Render(msg.Content)
	}
	fmt.Fprintf(sb, "%s", content)
}

func renderAssistantMessage(sb *strings.Builder, am domain.AssistantMessage, messages domain.Messages, idx int, displays domain.ToolDisplays, renderer ui.Renderer, theme *ui.Theme, width int, includeLeadingNewline bool) {
	printHeader := true
	if idx > 0 {
		prevRole := messages[idx-1].Role()
		if prevRole == domain.RoleAssistant || prevRole == domain.RoleTool {
			printHeader = false
		}
	}

	if printHeader {
		renderDivider(sb, theme, width, theme.MutedColor(), includeLeadingNewline)

		style := lipgloss.NewStyle().
			Foreground(theme.MutedColor()).
			Bold(true)

		// Divider already handles vertical spacing
		fmt.Fprintf(sb, "%s\n", style.Render("ASSISTANT:"))
	}

	if am.Content != "" {
		content := am.Content
		if renderer != nil {
			content = renderer.Render(am.Content)
		}
		fmt.Fprintf(sb, "%s", content)
	}

	for _, tc := range am.ToolCalls {
		display, ok := displays[tc.ID]
		if !ok {
			continue
		}

		// Status determination: look for the tool result message
		status := ui.StatusSuccess
		var toolOutput string
		var toolErr string

		for j := idx + 1; j < len(messages); j++ {
			if msgJ, ok := messages[j].(domain.ToolMessage); ok && msgJ.ToolCallID == tc.ID {
				toolOutput = msgJ.Content
				if msgJ.ToolError {
					status = ui.StatusError
					toolErr = strings.TrimPrefix(toolOutput, "Error:")
					toolErr = strings.TrimSpace(toolErr)
				}
				break
			}
		}

		// Use width-2 for the content to account for the box borders,
		// matching the logic in engine.go
		boxWidth := width - 2
		tooling := ui.NewToolRenderer(theme, width)
		prefix := tooling.StatusPrefix(status, "")
		var rendered string
		switch d := display.(type) {
		case domain.StringDisplay:
			rendered = tooling.RenderString(d, status, toolErr, prefix)
		case domain.DiffDisplay:
			rendered = tooling.RenderDiff(d, status, toolErr, prefix)
		case domain.ShellDisplay:
			// Prefer baked captured output over the decorated toolOutput (which includes exit codes for LLM)
			output := toolOutput
			if d.CapturedOutput != nil {
				output = *d.CapturedOutput
			}
			rendered = tooling.RenderShell(d, output, status, toolErr, prefix)
		}

		if rendered != "" {
			fmt.Fprint(sb, tooling.Box(rendered, boxWidth, status))
		}
	}
}
