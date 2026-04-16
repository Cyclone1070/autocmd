package history

import (
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/charmbracelet/lipgloss"
	"github.com/cloudwego/eino/schema"
)

const gutterWidth = 2 // "A│" / "U┃" / " │" (assistant) or " ┃" (user)

// userGutterPipe is BOX DRAWINGS HEAVY VERTICAL (thicker than the light │ used for assistant).
const userGutterPipe = "┃"

type renderItem struct {
	idx              int
	assistantIndices []int
}

func buildRenderItems(messages []*schema.Message) []renderItem {
	var items []renderItem
	for i := 0; i < len(messages); i++ {
		// Tool messages are represented via tool boxes attached to the preceding assistant message.
		// Rendering them as standalone entries would introduce extra blank lines and duplicate output.
		if messages[i].Role == schema.Tool {
			continue
		}

		// Coalesce consecutive assistant turns into a single rendered assistant block.
		if messages[i].Role == schema.Assistant {
			assistantIdxs := []int{i}
			j := i + 1
			for j < len(messages) {
				if messages[j].Role == schema.Tool {
					j++
					continue
				}
				if messages[j].Role != schema.Assistant {
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

// HistoryBuilder renders session history using a markdown renderer, theme, and terminal width.
type HistoryBuilder struct {
	Renderer ui.Renderer
	Theme    *ui.Theme
	Width    int
}

// NewHistoryBuilder returns a HistoryBuilder. Width is the full chat column width (including gutter).
func NewHistoryBuilder(renderer ui.Renderer, theme *ui.Theme, width int) *HistoryBuilder {
	return &HistoryBuilder{Renderer: renderer, Theme: theme, Width: width}
}

func (h *HistoryBuilder) contentWidth() int {
	cw := h.Width - gutterWidth
	if cw < 10 {
		return 10
	}
	return cw
}

// BuildSession renders the full session transcript.
func (h *HistoryBuilder) BuildSession(session *domain.Session) string {
	var sb strings.Builder
	messages := session.Messages
	displays := session.ToolDisplays

	items := buildRenderItems(messages)
	for renderedCount, it := range items {
		if len(it.assistantIndices) > 1 {
			sb.WriteString(h.renderCoalescedAssistant(messages, it.assistantIndices, displays))
			continue
		}
		sb.WriteString(h.RenderMessage(messages, it.idx, displays, renderedCount > 0))
	}

	return sb.String()
}

func (h *HistoryBuilder) renderCoalescedAssistant(messages []*schema.Message, assistantIndices []int, displays domain.ToolDisplays) string {
	var sb strings.Builder
	// Exactly one blank line before and after each message.
	sb.WriteString("\n")
	h.renderAssistantSequence(&sb, messages, assistantIndices, displays)
	sb.WriteString("\n")
	return sb.String()
}

func (h *HistoryBuilder) renderAssistantSequence(sb *strings.Builder, messages []*schema.Message, assistantIndices []int, displays domain.ToolDisplays) {
	style := lipgloss.NewStyle().Foreground(h.Theme.MutedColor()).Bold(true)
	roleLine := style.Render("A│")
	contPrefix := style.Render(" │")

	contentWidth := h.contentWidth()

	var parts []string
	for _, ai := range assistantIndices {
		am := messages[ai]
		if am.Role != schema.Assistant {
			continue
		}

		// Content at the point it was produced.
		if am.Content != "" {
			content := am.Content
			if h.Renderer != nil {
				content = h.Renderer.Render(am.Content)
			}
			parts = append(parts, content)
		}

		// Tool boxes exactly where tool calls occurred.
		for _, tc := range am.ToolCalls {
			rendered := h.renderToolCall(&tc, displays, contentWidth)
			if rendered != "" {
				parts = append(parts, rendered)
			}
		}
	}

	// Parts may already include intentional leading spacing (e.g. Theme.Box starts with "\n"),
	// so join with a single newline to avoid double-counting vertical gaps.
	body := strings.Join(parts, "\n")
	writeFramedWithGutter(sb, roleLine, contPrefix, body)
}

// RenderMessage renders a single message at the given index.
// If includeLeadingNewline is true, it prepends a newline before the divider.
func (h *HistoryBuilder) RenderMessage(messages []*schema.Message, idx int, displays domain.ToolDisplays, includeLeadingNewline bool) string {
	var sb strings.Builder
	msg := messages[idx]

	// Exactly one blank line before and after each message.
	// When messages are concatenated, this yields two blank lines between them.
	sb.WriteString("\n")

	switch msg.Role {
	case schema.User:
		h.renderUserMessage(&sb, msg)
	case schema.Assistant:
		h.renderAssistantMessage(&sb, msg, displays)
	}

	sb.WriteString("\n")
	return sb.String()
}

func (h *HistoryBuilder) renderUserMessage(sb *strings.Builder, msg *schema.Message) {
	style := lipgloss.NewStyle().Foreground(h.Theme.PrimaryColor()).Bold(true)
	roleLine := style.Render("U" + userGutterPipe)
	contPrefix := style.Render(" " + userGutterPipe)

	content := msg.Content
	if msg.Extra["iav/is_notification"] == true {
		content = "Tool Box"
	} else if h.Renderer != nil {
		content = h.Renderer.Render(msg.Content)
	}
	writeFramedWithGutter(sb, roleLine, contPrefix, content)
}

func (h *HistoryBuilder) renderAssistantMessage(sb *strings.Builder, am *schema.Message, displays domain.ToolDisplays) {
	style := lipgloss.NewStyle().Foreground(h.Theme.MutedColor()).Bold(true)
	roleLine := style.Render("A│")
	contPrefix := style.Render(" │")

	contentWidth := h.contentWidth()

	var parts []string
	if am.Content != "" {
		content := am.Content
		if h.Renderer != nil {
			content = h.Renderer.Render(am.Content)
		}
		parts = append(parts, content)
	}

	for _, tc := range am.ToolCalls {
		rendered := h.renderToolCall(&tc, displays, contentWidth)
		if rendered != "" {
			parts = append(parts, rendered)
		}
	}

	// Frame the assistant message exactly once, regardless of how many parts it has.
	// Parts may already include intentional leading spacing (e.g. Theme.Box starts with "\n"),
	// so join with a single newline to avoid double-counting vertical gaps.
	body := strings.Join(parts, "\n")
	writeFramedWithGutter(sb, roleLine, contPrefix, body)
}

func (h *HistoryBuilder) renderToolCall(tc *schema.ToolCall, displays domain.ToolDisplays, contentWidth int) string {
	display, ok := displays[tc.ID]
	if !ok {
		return ""
	}

	status := ui.StatusSuccess
	var toolErr string

	boxWidth := contentWidth - 2
	tooling := ui.NewToolRenderer(h.Theme, contentWidth, ui.NewToolOutputGater(12))
	prefix := tooling.StatusPrefix(status, "")
	var rendered string

	switch d := display.(type) {
	case domain.StringDisplay:
		if d.Error != "" {
			status = ui.StatusError
			toolErr = d.Error
			prefix = tooling.StatusPrefix(status, "")
		}
		rendered = tooling.RenderString(d, status, toolErr, prefix)
	case domain.DiffDisplay:
		if d.Error != "" {
			status = ui.StatusError
			toolErr = d.Error
			prefix = tooling.StatusPrefix(status, "")
		}
		rendered = tooling.RenderDiff(d, status, toolErr, prefix)
	case domain.BashDisplay:
		if d.Error != "" {
			status = ui.StatusError
			toolErr = d.Error
			prefix = tooling.StatusPrefix(status, "")
		}
		rendered = tooling.RenderBash(d, d.CapturedOutput, status, toolErr, prefix)
	}

	if rendered == "" {
		return ""
	}

	return tooling.Box(rendered, boxWidth, status)
}

// writeFramedWithGutter renders a symmetric, guttered frame:
// - roleLine (e.g. "U┃" or "A│") on its own line
// - one guttered blank line (" ┃" or " │")
// - the normalized content with every line prefixed by " ┃" or " │" (including blank lines)
// - one guttered blank line (" ┃" or " │") at the end
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
