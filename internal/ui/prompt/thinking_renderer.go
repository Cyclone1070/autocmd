package prompt

import (
	"fmt"
	"strings"
	"time"

	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/charmbracelet/lipgloss"
)

type thoughtContentGater interface {
	Gate(lines []string, scrollOffset int, scrollable bool, theme *ui.Theme) (gated []string, maxScroll int)
}

// ThinkingRenderer handles the "Thinking" state rendering.
type ThinkingRenderer struct {
	gater thoughtContentGater
	Theme *ui.Theme
	Width int
}

// NewThinkingRenderer creates a new ThinkingRenderer.
func NewThinkingRenderer(th *ui.Theme, width int, g thoughtContentGater) *ThinkingRenderer {
	if g == nil {
		g = ui.NewNoOpGater()
	}
	return &ThinkingRenderer{
		Theme: th,
		Width: width,
		gater: g,
	}
}

// RenderThinking renders a thinking line and optional live thought content.
func (r *ThinkingRenderer) RenderThinking(status ui.ToolStatus, start time.Time, tick int, thoughtText string, sp spinnerProvider) string {
	dur := time.Since(start).Round(time.Second).String()

	var label string

	switch status {
	case ui.StatusRunning:
		label = r.Theme.Primary(fmt.Sprintf("Thinking for %s", dur))
	case ui.StatusSuccess:
		label = r.Theme.Success(fmt.Sprintf("Thought for %s", dur))
	case ui.StatusError:
		label = r.Theme.Error(fmt.Sprintf("Thought for %s", dur))
	}

	spec := ui.ToolBlockSpec{
		Status:      status,
		Frame:       sp.Frame(tick),
		HeaderLines: []string{label},
	}
	headerPrefixWidth := lipgloss.Width(r.Theme.StatusPrefix(spec.Status, spec.Frame))
	headerContinuationWidth := r.Width - lipgloss.Width(ui.ToolInsetPrefix)
	headerFirstWidth := headerContinuationWidth - headerPrefixWidth
	spec.HeaderLines = wrapThinkingLines(spec.HeaderLines, headerFirstWidth, headerContinuationWidth)

	if status == ui.StatusRunning {
		contentFirstWidth := r.Width - lipgloss.Width(ui.ToolInsetPrefix+ui.ToolFirstContentGutterPrefix)
		contentContinuationWidth := r.Width - lipgloss.Width(ui.ToolInsetPrefix+ui.ToolContentGutterPrefix)
		wrappedContent := completedVisualThoughtLines(thoughtText, contentFirstWidth, contentContinuationWidth)
		wrappedContent, _ = r.gater.Gate(wrappedContent, 0, false, r.Theme)
		for i := range wrappedContent {
			wrappedContent[i] = r.Theme.Muted(wrappedContent[i])
		}
		spec.ContentLines = wrappedContent
	}
	return r.Theme.RenderToolBlock(spec)
}

func completedVisualThoughtLines(text string, firstWidth, continuationWidth int) []string {
	if text == "" {
		return nil
	}
	endedWithNewline := strings.HasSuffix(text, "\n")
	text = strings.TrimSuffix(text, "\n")
	logicalLines := strings.Split(text, "\n")
	wrapped := wrapThinkingLines(logicalLines, firstWidth, continuationWidth)
	if endedWithNewline {
		return wrapped
	}
	if len(wrapped) == 0 {
		return nil
	}
	return wrapped[:len(wrapped)-1]
}

func wrapThinkingLines(lines []string, firstWidth, continuationWidth int) []string {
	if len(lines) == 0 {
		return nil
	}
	if firstWidth < 1 {
		firstWidth = 1
	}
	if continuationWidth < 1 {
		continuationWidth = 1
	}

	var out []string
	for i, line := range lines {
		width := continuationWidth
		if i == 0 {
			width = firstWidth
		}
		if line == "" {
			out = append(out, "")
			continue
		}
		wrapped := lipgloss.NewStyle().Width(width).Render(line)
		for part := range strings.SplitSeq(wrapped, "\n") {
			out = append(out, strings.TrimRight(part, " "))
		}
	}
	return out
}
