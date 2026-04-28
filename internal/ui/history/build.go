// Package history provides components for rendering the conversation history in the terminal.
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
	idx                int
	assistantIndices   []int
	assistantCancelled bool // session was cancelled after this assistant block; show gutter marker only (no cancel text).
}

func buildRenderItems(messages []*schema.Message) []renderItem {
	var items []renderItem
	for i := 0; i < len(messages); i++ {
		// Tool messages are represented via tool boxes attached to the preceding assistant message.
		// Rendering them as standalone entries would introduce extra blank lines and duplicate output.
		if messages[i].Role == schema.Tool {
			continue
		}
		if messages[i].Extra[domain.NotificationMessageExtraKey] == true {
			continue
		}
		if messages[i].Extra[domain.CancelMessageExtraKey] == true {
			continue
		}

		// Coalesce consecutive assistant turns into a single rendered assistant block.
		if messages[i].Role == schema.Assistant {
			assistantIdxs := []int{i}
			assistantCancelled := false
			j := i + 1
			for j < len(messages) {
				if messages[j].Role == schema.Tool {
					j++
					continue
				}
				if messages[j].Extra[domain.NotificationMessageExtraKey] == true {
					j++
					continue
				}
				if messages[j].Extra[domain.CancelMessageExtraKey] == true {
					assistantCancelled = true
					j++
					continue
				}
				if messages[j].Role != schema.Assistant {
					break
				}
				assistantIdxs = append(assistantIdxs, j)
				j++
			}
			if len(assistantIdxs) > 1 || assistantCancelled {
				items = append(items, renderItem{idx: i, assistantIndices: assistantIdxs, assistantCancelled: assistantCancelled})
				i = j - 1
				continue
			}
		}

		items = append(items, renderItem{idx: i})
	}
	return items
}

// Builder renders session history using a markdown renderer, theme, and terminal width.
type Builder struct {
	Renderer         ui.Renderer
	Theme            *ui.Theme
	Width            int
	BashOutputHeight int
}

// NewBuilder returns a Builder. Width is the full chat column width (including gutter).
func NewBuilder(renderer ui.Renderer, theme *ui.Theme, width int, bashOutputHeight int) *Builder {
	return &Builder{Renderer: renderer, Theme: theme, Width: width, BashOutputHeight: bashOutputHeight}
}

func (h *Builder) contentWidth() int {
	return h.Width - gutterWidth
}

// BuildSession renders the full session transcript.
func (h *Builder) BuildSession(session *domain.Session) string {
	var sb strings.Builder
	messages := session.Messages
	displays := session.ToolDisplays

	items := buildRenderItems(messages)
	for renderedCount, it := range items {
		if len(it.assistantIndices) > 0 {
			sb.WriteString(h.renderCoalescedAssistant(messages, it.assistantIndices, displays, it.assistantCancelled))
			continue
		}
		sb.WriteString(h.RenderMessage(messages, it.idx, displays, renderedCount > 0))
	}

	return sb.String()
}

func (h *Builder) renderCoalescedAssistant(messages []*schema.Message, assistantIndices []int, displays domain.ToolDisplays, assistantCancelled bool) string {
	var sb strings.Builder
	// Exactly one blank line before and after each message.
	sb.WriteString("\n")
	h.renderAssistantSequence(&sb, messages, assistantIndices, displays, assistantCancelled)
	sb.WriteString("\n")
	return sb.String()
}

func (h *Builder) renderAssistantSequence(sb *strings.Builder, messages []*schema.Message, assistantIndices []int, displays domain.ToolDisplays, assistantCancelled bool) {
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
			parts = append(parts, ui.NormalizeBlock(content))
		}

		// Tool boxes exactly where tool calls occurred.
		if len(am.ToolCalls) > 0 {
			var toolBoxes []string
			for _, tc := range am.ToolCalls {
				rendered := h.renderToolCall(&tc, displays, contentWidth)
				if rendered != "" {
					toolBoxes = append(toolBoxes, rendered)
				}
			}
			if len(toolBoxes) > 0 {
				toolBlock := strings.Join(toolBoxes, "")
				parts = append(parts, ui.NormalizeBlock(toolBlock))
			}
		}
	}

	body := strings.Join(parts, "\n")
	h.writeAssistantFramedWithGutter(sb, body, assistantCancelled)
}

// RenderMessage renders a single message at the given index.
// If includeLeadingNewline is true, it prepends a newline before the divider.
func (h *Builder) RenderMessage(messages []*schema.Message, idx int, displays domain.ToolDisplays, _ bool) string {
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

func (h *Builder) renderUserMessage(sb *strings.Builder, msg *schema.Message) {
	style := lipgloss.NewStyle().Foreground(h.Theme.PrimaryColor()).Bold(true)
	roleLine := style.Render("U" + userGutterPipe)
	contPrefix := style.Render(" " + userGutterPipe)

	content := msg.Content
	if h.Renderer != nil {
		content = h.Renderer.Render(msg.Content)
	}
	writeFramedWithGutter(sb, roleLine, contPrefix, ui.NormalizeBlock(content))
}

func (h *Builder) renderAssistantMessage(sb *strings.Builder, am *schema.Message, displays domain.ToolDisplays) {
	contentWidth := h.contentWidth()

	var parts []string
	if am.Content != "" {
		content := am.Content
		if h.Renderer != nil {
			content = h.Renderer.Render(am.Content)
		}
		parts = append(parts, ui.NormalizeBlock(content))
	}

	if len(am.ToolCalls) > 0 {
		var toolBoxes []string
		for _, tc := range am.ToolCalls {
			rendered := h.renderToolCall(&tc, displays, contentWidth)
			if rendered != "" {
				toolBoxes = append(toolBoxes, rendered)
			}
		}
		if len(toolBoxes) > 0 {
			toolBlock := strings.Join(toolBoxes, "")
			parts = append(parts, ui.NormalizeBlock(toolBlock))
		}
	}

	// Join normalized parts with a single newline (as terminal surrogate).
	body := strings.Join(parts, "\n")
	h.writeAssistantFramedWithGutter(sb, body, false)
}

func (h *Builder) renderToolCall(tc *schema.ToolCall, displays domain.ToolDisplays, contentWidth int) string {
	display, ok := displays[tc.ID]
	if !ok {
		return ""
	}

	status := ui.StatusSuccess
	var toolErr string

	tooling := ui.NewToolRenderer(h.Theme, contentWidth, ui.NewToolOutputGater(h.BashOutputHeight))
	frame := ""
	var rendered string

	switch d := display.(type) {
	case domain.StringDisplay:
		if d.Error != "" {
			status = ui.StatusError
			toolErr = d.Error
		}
		rendered = tooling.RenderString(d, status, toolErr, frame)
	case domain.DiffDisplay:
		if d.Error != "" {
			status = ui.StatusError
			toolErr = d.Error
		}
		rendered = tooling.RenderDiff(d, status, toolErr, frame)
	case domain.BashDisplay:
		if d.Error != "" {
			status = ui.StatusError
			toolErr = d.Error
		}
		rendered = tooling.RenderBash(d, d.CapturedOutput, status, toolErr, frame)
	case domain.QuestionDisplay:
		rendered = tooling.RenderQuestion(d, ui.NewQuestionUIState(d), status, toolErr, frame)
	}

	if rendered == "" {
		return ""
	}

	return rendered
}

// writeAssistantFramedWithGutter frames assistant content like writeFramedWithGutter, but when
// assistantCancelled the bottom symmetric gutter line becomes a red ✘ in the role column (cancel
// text stays in session messages for the model; history shows this marker only).
func (h *Builder) writeAssistantFramedWithGutter(sb *strings.Builder, body string, assistantCancelled bool) {
	style := lipgloss.NewStyle().Foreground(h.Theme.MutedColor()).Bold(true)
	roleLine := style.Render("A│")
	contPrefix := style.Render(" │")

	content := strings.Trim(body, "\n")
	if content == "" {
		sb.WriteString(roleLine)
		sb.WriteString("\n")
		if assistantCancelled {
			sb.WriteString(h.assistantCancelGutterLine())
		} else {
			sb.WriteString(contPrefix)
		}
		return
	}

	lines := strings.Split(content, "\n")
	sb.WriteString(roleLine)
	sb.WriteString("\n")
	for i, line := range lines {
		sb.WriteString(contPrefix)
		sb.WriteString(line)
		if i < len(lines)-1 {
			sb.WriteString("\n")
		}
	}
	sb.WriteString("\n")
	if assistantCancelled {
		sb.WriteString(h.assistantCancelGutterLine())
	} else {
		sb.WriteString(contPrefix)
	}
}

func (h *Builder) assistantCancelGutterLine() string {
	errSt := lipgloss.NewStyle().Foreground(h.Theme.ErrorColor()).Bold(true)
	muted := lipgloss.NewStyle().Foreground(h.Theme.MutedColor()).Bold(true)
	return errSt.Render("✘") + muted.Render("│")
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
