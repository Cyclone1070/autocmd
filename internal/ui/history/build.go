package history

import (
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/charmbracelet/lipgloss"
)

const gutterWidth = 2 // "A│" / "U│" / " │"

type renderItem struct {
	idx              int
	assistantIndices []int
}

func buildRenderItems(messages domain.Messages) []renderItem {
	var items []renderItem
	for i := 0; i < len(messages); i++ {
		// Tool messages are represented via tool boxes attached to the preceding assistant message.
		// Rendering them as standalone entries would introduce extra blank lines and duplicate output.
		if messages[i].Role() == domain.RoleTool {
			continue
		}

		// Coalesce consecutive assistant turns into a single rendered assistant block.
		// IMPORTANT: preserve chronological order: tool boxes must render where the tool call occurred,
		// not always after the final merged text.
		if _, ok := messages[i].(domain.AssistantMessage); ok {
			assistantIdxs := []int{i}
			j := i + 1
			for j < len(messages) {
				if messages[j].Role() == domain.RoleTool {
					j++
					continue
				}
				if _, ok := messages[j].(domain.AssistantMessage); !ok {
					break
				}
				assistantIdxs = append(assistantIdxs, j)
				j++
			}
			if len(assistantIdxs) > 1 {
				items = append(items, renderItem{idx: i, assistantIndices: assistantIdxs})
				i = j - 1
				continue
			}
		}

		items = append(items, renderItem{idx: i})
	}
	return items
}

// BuildHistory constructs a pre-rendered string representation of the session history.
func BuildHistory(session *domain.Session, renderer ui.Renderer, theme *ui.Theme, width int) string {
	var sb strings.Builder
	messages := session.Messages
	displays := session.ToolDisplays

	items := buildRenderItems(messages)
	for renderedCount, it := range items {
		if len(it.assistantIndices) > 1 {
			sb.WriteString(renderCoalescedAssistant(messages, it.assistantIndices, displays, renderer, theme, width, renderedCount > 0))
			continue
		}
		sb.WriteString(RenderMessage(messages, it.idx, displays, renderer, theme, width, renderedCount > 0))
	}

	return sb.String()
}

// renderMessageStandalone renders a single-message slice, ensuring spacing rules are applied
// (used for coalesced assistant messages).
func renderMessageStandalone(messages domain.Messages, displays domain.ToolDisplays, renderer ui.Renderer, theme *ui.Theme, width int, includeLeadingNewline bool) string {
	return RenderMessage(messages, 0, displays, renderer, theme, width, includeLeadingNewline)
}

func renderCoalescedAssistant(messages domain.Messages, assistantIndices []int, displays domain.ToolDisplays, renderer ui.Renderer, theme *ui.Theme, width int, includeLeadingNewline bool) string {
	var sb strings.Builder
	// Exactly one blank line before and after each message.
	sb.WriteString("\n")
	renderAssistantSequence(&sb, messages, assistantIndices, displays, renderer, theme, width)
	sb.WriteString("\n")
	return sb.String()
}

func renderAssistantSequence(sb *strings.Builder, messages domain.Messages, assistantIndices []int, displays domain.ToolDisplays, renderer ui.Renderer, theme *ui.Theme, width int) {
	style := lipgloss.NewStyle().Foreground(theme.MutedColor()).Bold(true)
	roleLine := style.Render("A│")
	contPrefix := style.Render(" │")

	contentWidth := width - gutterWidth
	if contentWidth < 10 {
		contentWidth = 10
	}

	var parts []string
	for _, ai := range assistantIndices {
		am, ok := messages[ai].(domain.AssistantMessage)
		if !ok {
			continue
		}

		// Content at the point it was produced.
		if am.Content != "" {
			content := am.Content
			if renderer != nil {
				content = renderer.Render(am.Content)
			}
			parts = append(parts, content)
		}

		// Tool boxes exactly where tool calls occurred.
		for _, tc := range am.ToolCalls {
			display, ok := displays[tc.ID]
			if !ok {
				continue
			}

			status := ui.StatusSuccess
			var toolOutput string
			var toolErr string

			for j := ai + 1; j < len(messages); j++ {
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

			boxWidth := contentWidth - 2
			tooling := ui.NewToolRenderer(theme, contentWidth, ui.NewToolOutputGater(12))
			prefix := tooling.StatusPrefix(status, "")
			var rendered string
			switch d := display.(type) {
			case domain.StringDisplay:
				rendered = tooling.RenderString(d, status, toolErr, prefix)
			case domain.DiffDisplay:
				rendered = tooling.RenderDiff(d, status, toolErr, prefix)
			case domain.ShellDisplay:
				output := toolOutput
				if d.CapturedOutput != nil {
					output = *d.CapturedOutput
				}
				rendered = tooling.RenderShell(d, output, status, toolErr, prefix)
			}

			if rendered != "" {
				toolBox := tooling.Box(rendered, boxWidth, status)
				parts = append(parts, toolBox)
			}
		}
	}

	body := strings.Join(parts, "\n\n")
	writeFramedWithGutter(sb, roleLine, contPrefix, body)
}

// RenderMessage renders a single message at the given index.
// If includeLeadingNewline is true, it prepends a newline before the divider.
func RenderMessage(messages domain.Messages, idx int, displays domain.ToolDisplays, renderer ui.Renderer, theme *ui.Theme, width int, includeLeadingNewline bool) string {
	var sb strings.Builder
	msg := messages[idx]

	// Exactly one blank line before and after each message.
	// When messages are concatenated, this yields two blank lines between them.
	sb.WriteString("\n")

	switch m := msg.(type) {
	case domain.UserMessage:
		renderUserMessage(&sb, m, renderer, theme, width)
	case domain.AssistantMessage:
		renderAssistantMessage(&sb, m, messages, idx, displays, renderer, theme, width)
	}

	sb.WriteString("\n")
	return sb.String()
}

func renderUserMessage(sb *strings.Builder, msg domain.UserMessage, renderer ui.Renderer, theme *ui.Theme, width int) {
	_ = width
	style := lipgloss.NewStyle().Foreground(theme.PrimaryColor()).Bold(true)
	roleLine := style.Render("U│")
	contPrefix := style.Render(" │")

	content := msg.Content
	if renderer != nil {
		content = renderer.Render(msg.Content)
	}
	writeFramedWithGutter(sb, roleLine, contPrefix, content)
}

func renderAssistantMessage(sb *strings.Builder, am domain.AssistantMessage, messages domain.Messages, idx int, displays domain.ToolDisplays, renderer ui.Renderer, theme *ui.Theme, width int) {
	style := lipgloss.NewStyle().Foreground(theme.MutedColor()).Bold(true)
	roleLine := style.Render("A│")
	contPrefix := style.Render(" │")

	contentWidth := width - gutterWidth
	if contentWidth < 10 {
		contentWidth = 10
	}

	var parts []string
	if am.Content != "" {
		content := am.Content
		if renderer != nil {
			content = renderer.Render(am.Content)
		}
		parts = append(parts, content)
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
		boxWidth := contentWidth - 2
		tooling := ui.NewToolRenderer(theme, contentWidth, ui.NewToolOutputGater(12))
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
			toolBox := tooling.Box(rendered, boxWidth, status)
			parts = append(parts, toolBox)
		}
	}

	// Frame the assistant message exactly once, regardless of how many parts it has.
	body := strings.Join(parts, "\n\n")
	writeFramedWithGutter(sb, roleLine, contPrefix, body)
}

// writeFramedWithGutter renders a symmetric, guttered frame:
// - roleLine (e.g. "U│" or "A│") on its own line
// - one guttered blank line (" │")
// - the normalized content with every line prefixed by " │" (including blank lines)
// - one guttered blank line (" │") at the end
//
// The only unguttered blank lines should come from RenderMessage's outer padding.
func writeFramedWithGutter(sb *strings.Builder, roleLine, contPrefix, content string) {
	// Normalize leading/trailing newlines so renderer/toolbox can't affect inter-message spacing.
	content = strings.Trim(content, "\n")
	if content == "" {
		// Empty message: just role line and bottom guttered blank line (symmetry).
		sb.WriteString(roleLine)
		sb.WriteString("\n")
		sb.WriteString(contPrefix)
		return
	}

	lines := strings.Split(content, "\n")

	// Role line
	sb.WriteString(roleLine)
	sb.WriteString("\n")

	// Body (all lines guttered)
	for i, line := range lines {
		sb.WriteString(contPrefix)
		sb.WriteString(line)
		if i < len(lines)-1 {
			sb.WriteString("\n")
		}
	}

	// Bottom guttered blank line (symmetry with role line).
	sb.WriteString("\n")
	sb.WriteString(contPrefix)
}
