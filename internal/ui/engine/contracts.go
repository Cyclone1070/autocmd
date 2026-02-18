package engine

import "github.com/Cyclone1070/iav/internal/ui/theme"

// MarkdownStream handles append/pending/renderRemaining for streaming markdown.
// Consumer-defined; implemented by markdown package.
type MarkdownStream interface {
	Append(chunk string) (flushedBlocks []string, err error)
	Pending() string
	RenderRemaining() (string, error)
}

// ThemeAdapter provides style functions for rendering.
// Consumer-defined; implemented by ui/theme.
type ThemeAdapter interface {
	Success(s string) string
	Error(s string) string
	Muted(s string) string
	Primary(s string) string
	Box(content string, width int, status theme.ToolStatus) string
	Separator(width int, status theme.ToolStatus) string
}

// LayoutAdapter provides truncation and padding calculations.
type LayoutAdapter interface {
	TruncateWithIndicator(content string, termHeight int) string
}

// ToolRenderer renders tool displays for the UI.
// Consumer-defined; implemented by compose package.
type ToolRenderer interface {
	Render(t *ToolState) string
}
