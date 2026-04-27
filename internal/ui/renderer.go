package ui

import (
	"log/slog"
	"regexp"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
)

const (
	codeStartMarker = "_IAV_CODE_START_"
	codeEndMarker   = "_IAV_CODE_END_"
)

// Renderer renders markdown to ANSI strings. Failure should be handled internally.
type Renderer interface {
	Render(markdown string) string
}

// GlamourRenderer wraps glamour.TermRenderer to implement Renderer.
type GlamourRenderer struct {
	tr     *glamour.TermRenderer
	isDark bool
}

var reANSI = regexp.MustCompile(`\x1b\[[0-9;]*[mGKH]`)

// Render implements Renderer. On error, it returns the original markdown and logs.
func (g *GlamourRenderer) Render(markdown string) string {
	rendered, err := g.tr.Render(markdown)
	if err != nil {
		slog.Warn("glamour render failed, falling back to raw text", "err", err)
		return markdown
	}

	// Post-process ONLY content between markers to add the red bar.
	// We capture the entire line prefix to ensure absolute alignment with glamour's document layout.
	re := regexp.MustCompile(`(?sm)^([^\n]*?)` + regexp.QuoteMeta(codeStartMarker) + `(.*?)\n?^([^\n]*?)` + regexp.QuoteMeta(codeEndMarker))

	// Red color for the bar
	red := "\x1b[38;2;240;93;94m" // #F05D5E (Light)
	if g.isDark {
		red = "\x1b[38;2;255;102;102m" // #FF6666 (Dark)
	}
	bar := red + "┃" + "\x1b[0m"

	out := re.ReplaceAllStringFunc(rendered, func(match string) string {
		sub := re.FindStringSubmatch(match)
		if len(sub) < 4 {
			return match
		}
		prefix := sub[1]
		if prefix == "" {
			prefix = "  " // Fallback to standard document margin if marker was at start of line
		}
		content := sub[2]
		lines := strings.Split(content, "\n")

		// Helper to check if a line is blank (ignoring ANSI codes)
		isBlank := func(s string) bool {
			stripped := reANSI.ReplaceAllString(s, "")
			return strings.TrimSpace(stripped) == ""
		}

		// Trim leading/trailing empty lines from the code block itself
		start := 0
		for start < len(lines) && isBlank(lines[start]) {
			start++
		}
		end := len(lines)
		for end > start && isBlank(lines[end-1]) {
			end--
		}
		coreLines := lines[start:end]

		// Build the barred block
		var result []string
		for i, line := range coreLines {
			// Remove glamour's right-side padding before adding our bar
			trimmedLine := strings.TrimRight(line, " ")

			// Normalize indentation: Glamour/Goldmark often adds 2 spaces of ghost
			// indentation to lines 2+ of code blocks but not the first line.
			// These spaces can be preceded by ANSI escape codes.
			if i > 0 {
				reGhost := regexp.MustCompile(`^(\x1b\[[0-9;]*[mGKH])*  `)
				trimmedLine = reGhost.ReplaceAllString(trimmedLine, "$1")
			}

			result = append(result, prefix+bar+" "+trimmedLine)
		}

		return strings.Join(result, "\n") + "\n"
	})

	return strings.TrimRight(out, "\n")
}

// PassthroughRenderer is a no-op renderer that returns markdown as-is.
type PassthroughRenderer struct{}

func (p *PassthroughRenderer) Render(markdown string) string {
	return markdown
}

// NewGlamourRenderer creates a Renderer using glamour with the given width.
// If glamour initialization fails, it logs the error and returns a PassthroughRenderer.
func NewGlamourRenderer(width int, isDark bool) Renderer {
	var style ansi.StyleConfig
	if isDark {
		style = styles.DarkStyleConfig
	} else {
		style = styles.LightStyleConfig
	}

	// Remove background colors to ensure safe contrast on all palettes
	style.H1.BackgroundColor = nil
	style.Code.BackgroundColor = nil

	// Match H1 foreground to other headings (H2) and restore bold/underline
	style.H1.Color = style.H2.Color
	style.H1.Bold = new(true)
	style.H1.Underline = new(true)
	style.H1.Upper = new(true)

	// Disable document end padding
	style.Document.BlockSuffix = ""

	// Set markers and disable default indentation/margin for precise post-processing
	style.CodeBlock.BlockPrefix = codeStartMarker
	style.CodeBlock.BlockSuffix = codeEndMarker
	zero := uint(0)
	style.CodeBlock.Indent = &zero
	style.CodeBlock.Margin = &zero

	tr, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		slog.Warn("failed to create glamour renderer, using passthrough", "err", err)
		return &PassthroughRenderer{}
	}
	return &GlamourRenderer{tr: tr, isDark: isDark}
}
