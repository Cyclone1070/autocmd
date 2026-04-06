// Package tool provides rendering for tool outputs (StringDisplay, DiffDisplay, ShellDisplay).
// Used by compose when wiring engine.Deps.ViewTool.

package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/charmbracelet/lipgloss"
)

type gater interface {
	Gate(content string) string
}

// ToolRenderer provides rendering for tool outputs (StringDisplay, DiffDisplay, ShellDisplay).
type ToolRenderer struct {
	Theme *Theme
	Width int
	gater gater
}

// NewToolRenderer creates a new ToolRenderer.
func NewToolRenderer(theme *Theme, width int, g gater) *ToolRenderer {
	return &ToolRenderer{
		Theme: theme,
		Width: width,
		gater: g,
	}
}

func (r *ToolRenderer) SetShortToolbox(b bool) {
	r.Theme.ShortToolbox = b
}

// StatusPrefix returns a styled and padded status indicator.
func (r *ToolRenderer) StatusPrefix(status ToolStatus, frame string) string {
	return r.Theme.StatusPrefix(status, frame)
}

// Pad adds the status prefix to the first line and standard indentation to others.
func (r *ToolRenderer) Pad(text, prefix string) string {
	if text == "" {
		return ""
	}

	// Calculate the available width for content after borders (2), padding (2), and prefix.
	// Total overhead is 4 (2 for borders + 2 for horizontal padding defined in theme.go).
	prefixWidth := lipgloss.Width(prefix)
	contentWidth := r.Width - 4 - prefixWidth

	// Wrap and style the content as a vertical column.
	contentStyle := lipgloss.NewStyle().Width(contentWidth)
	wrappedContent := contentStyle.Render(text)

	// Join the prefix and the content column horizontally.
	// lipgloss.Top ensures the prefix is only aligned with the first line of content.
	return lipgloss.JoinHorizontal(lipgloss.Top, prefix, wrappedContent)
}

func (r *ToolRenderer) formatError(prefix string, err string, isMuted bool) string {
	if prefix == "" {
		return r.Theme.Error(err)
	}
	separator := " - "
	if isMuted {
		return r.Theme.Muted(prefix+separator) + r.Theme.Error(err)
	}
	return prefix + separator + r.Theme.Error(err)
}

// RenderString renders StringDisplay.
func (r *ToolRenderer) RenderString(d domain.StringDisplay, status ToolStatus, err string, prefix string) string {
	var parts []string

	if d.Comment != "" {
		header := "# " + d.Comment
		if status == StatusError && err != "" {
			header = r.formatError(header, err, true)
		} else {
			header = r.Theme.Muted(header)
		}
		parts = append(parts, header)
		if d.Content != "" {
			parts = append(parts, d.Content)
		}
	} else if d.Content != "" {
		content := d.Content
		if status == StatusError && err != "" {
			lines := strings.Split(content, "\n")
			lines[0] = r.formatError(lines[0], err, false)
			content = strings.Join(lines, "\n")
		}
		parts = append(parts, content)
	} else if status == StatusError && err != "" {
		parts = append(parts, r.Theme.Error(err))
	}

	if len(parts) == 0 {
		return ""
	}

	return r.Pad(strings.Join(parts, "\n\n"), prefix)
}

// RenderDiff renders DiffDisplay.
func (r *ToolRenderer) RenderDiff(d domain.DiffDisplay, status ToolStatus, err string, prefix string) string {
	header := d.Comment
	target := d.Target
	if target == "" {
		target = header
	}

	if status == StatusError {
		header = r.formatError("# "+header, err, true)
		parts := []string{header, target}
		return r.Pad(strings.Join(parts, "\n\n"), prefix)
	}

	// Add stats to target if success
	if status == StatusSuccess && (d.Added != 0 || d.Removed != 0) {
		target = fmt.Sprintf("%s (%s, %s)",
			target,
			r.Theme.Success(fmt.Sprintf("+%d", d.Added)),
			r.Theme.Error(fmt.Sprintf("-%d", d.Removed)))
	}

	header = r.Theme.Muted("# " + header)
	diffContent := r.colorizeDiff(d.Diff)
	diffContent = r.gater.Gate(diffContent)

	// Build parts with blank line separation
	parts := []string{
		header,
		target,
	}
	if !r.Theme.ShortToolbox {
		parts = append(parts, diffContent)
	}

	content := strings.Join(parts, "\n\n")
	return r.Pad(content, prefix)
}

func (r *ToolRenderer) colorizeDiff(diff string) string {
	lines := strings.Split(diff, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "+") {
			lines[i] = r.Theme.Success(line)
		} else if strings.HasPrefix(line, "-") {
			lines[i] = r.Theme.Error(line)
		}
	}
	return strings.Join(lines, "\n")
}

// RenderShell renders ShellDisplay.
func (r *ToolRenderer) RenderShell(d domain.ShellDisplay, output string, status ToolStatus, err string, prefix string) string {
	header := d.Comment
	if status == StatusError {
		header = r.formatError("# "+header, err, true)
		cmdLine := fmt.Sprintf("$ %s", d.Command)
		content := strings.Join([]string{header, cmdLine}, "\n\n")
		return r.Pad(content, prefix)
	}

	header = r.Theme.Muted("# " + header)
	cmdLine := fmt.Sprintf("$ %s", d.Command)

	shellOutput := r.gater.Gate(strings.TrimRight(output, "\n"))

	// Build parts with blank line separation
	parts := []string{
		header,
		cmdLine,
	}
	if shellOutput != "" && !r.Theme.ShortToolbox {
		parts = append(parts, shellOutput)
	}

	content := strings.Join(parts, "\n\n")
	return r.Pad(content, prefix)
}

// RenderQuestion renders QuestionDisplay with interactive state (cursor, toggles, custom text).
// Unlike other tool renderers, it never prepends a status prefix (spinner/checkmark); the question header is the first line.
func (r *ToolRenderer) RenderQuestion(d domain.QuestionDisplay, state QuestionUIState, status ToolStatus, err string) string {
	if len(d.Questions) == 0 {
		return ""
	}

	n := len(d.Questions)
	maxA := maxActiveIndex(d)
	active := state.Active
	if active < 0 || active > maxA {
		active = 0
	}
	stRender := state
	stRender.Active = active
	if stRender.ReviewCursor < 0 {
		stRender.ReviewCursor = 0
	}
	if stRender.ReviewCursor > 1 {
		stRender.ReviewCursor = 1
	}

	if n > 1 && active == n {
		answered := AnsweredQuestionCount(d, stRender.Per)
		unanswered := n - answered
		var plainSummary string
		if answered == n {
			plainSummary = "All questions answered"
		} else {
			indices := UnansweredIndices(d, stRender.Per)
			var strIndices []string
			for _, idx := range indices {
				strIndices = append(strIndices, strconv.Itoa(idx))
			}
			suffix := strings.Join(strIndices, ", ")
			if unanswered == 1 {
				plainSummary = fmt.Sprintf("1 question unanswered: %s", suffix)
			} else {
				plainSummary = fmt.Sprintf("%d questions unanswered: %s", unanswered, suffix)
			}
		}

		var summary string
		if status == StatusError && err != "" {
			summary = r.formatError(plainSummary, err, true)
		} else if answered == n {
			summary = r.Theme.Success("All questions answered")
		} else {
			summary = r.Theme.Error(plainSummary)
		}
		body := summary + "\n\n" + r.renderQuestionReviewBlock(stRender)
		body += "\n\n" + r.renderQuestionFooter(status, d.Questions[0], QuestionPerState{})
		return r.Pad(body, "")
	}

	cur := active + 1
	q := d.Questions[active]
	st := stRender.Per[active]

	plainHeader := fmt.Sprintf("Question %d/%d", cur, n)
	var header string
	if status == StatusError && err != "" {
		header = r.formatError(plainHeader, err, true)
	} else {
		numColor := r.Theme.Primary
		if QuestionHasAnswer(q, st) {
			numColor = r.Theme.Success
		}
		// Highlight current index (e.g. the "2" in "Question 2/3").
		header = r.Theme.Muted("Question ") +
			numColor(fmt.Sprintf("%d", cur)) +
			r.Theme.Muted("/") +
			r.Theme.Muted(fmt.Sprintf("%d", n))
	}

	parts := []string{header}
	parts = append(parts, q.Question)
	parts = append(parts, r.renderQuestionOptionBlock(q, st))

	body := strings.Join(parts, "\n\n")
	body += "\n\n" + r.renderQuestionFooter(status, q, st)
	return r.Pad(body, "")
}

func (r *ToolRenderer) renderQuestionReviewBlock(st QuestionUIState) string {
	opts := []string{"Submit", "Go back"}
	defFG := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	numColW := 2
	var lines []string
	for i, opt := range opts {
		rawNum := fmt.Sprintf("%d.", i+1)
		num := r.Theme.Muted(fmt.Sprintf("%*s", numColW, rawNum))
		var bullet, label string
		if st.ReviewCursor == i {
			bullet = r.Theme.Primary("●")
			label = r.Theme.Success(opt)
		} else {
			bullet = " "
			label = defFG.Render(opt)
		}
		lines = append(lines, bullet+" "+num+" "+label)
	}
	return strings.Join(lines, "\n")
}

// questionInnerWidth is the printable width for one line inside the tool box. The prompt uses
// boxWidth = max(width-2,1); lipgloss applies border + horizontal padding; empirically boxW-3
// matches the last column of inner text flush with the box padding (boxW-4 left a 1-cell gap).
func (r *ToolRenderer) questionInnerWidth() int {
	boxW := max(r.Width-2, 1)
	inner := boxW - 3
	if inner < 12 {
		return 12
	}
	return inner
}

func (r *ToolRenderer) renderQuestionFooter(status ToolStatus, _ domain.QuestionInfo, _ QuestionPerState) string {
	inner := r.questionInnerWidth()
	sep := r.Theme.Separator(inner, status)

	// Key (primary) + label (muted), with gaps between groups like the toolbox hint row.
	keyLabel := func(key, label string) string {
		return r.Theme.Primary(key) + " " + r.Theme.Muted(label)
	}
	gap := "   "
	row := keyLabel("↑/↓/←/→", "navigate") + gap +
		keyLabel("Enter", "select") + gap +
		keyLabel("c", "custom") + gap +
		keyLabel("q", "quit") + gap +
		keyLabel("s", "submit")
	if lipgloss.Width(row) > inner {
		gap = "  "
		row = keyLabel("↑/↓/←/→", "navigate") + gap +
			keyLabel("Enter", "select") + gap +
			keyLabel("c", "custom") + gap +
			keyLabel("q", "quit") + gap +
			keyLabel("s", "submit")
	}
	if lipgloss.Width(row) > inner {
		gap = " "
		row = keyLabel("↑/↓/←/→", "navigate") + gap +
			keyLabel("Enter", "select") + gap +
			keyLabel("c", "custom") + gap +
			keyLabel("q", "quit") + gap +
			keyLabel("s", "submit")
	}
	return sep + "\n" + row
}

func (r *ToolRenderer) renderQuestionOptionBlock(q domain.QuestionInfo, st QuestionPerState) string {
	lastNum := len(q.Options)
	if customRowVisible(st) {
		lastNum++
	}
	numColW := len(strconv.Itoa(lastNum)) + 1 // digits + '.'

	formatNum := func(n int) string {
		raw := fmt.Sprintf("%d.", n)
		return r.Theme.Muted(fmt.Sprintf("%*s", numColW, raw))
	}

	defFG := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))

	var lines []string
	for i, opt := range q.Options {
		n := i + 1
		num := formatNum(n)
		var bullet string
		if st.Cursor == i && !st.CustomInputFocused {
			bullet = r.Theme.Primary("●")
		} else {
			bullet = " "
		}
		main := opt
		selected := false
		if q.Multiple {
			mark := "[ ]"
			if i < len(st.MultiSelected) && st.MultiSelected[i] {
				mark = "[x]"
				selected = true
			}
			main = fmt.Sprintf("%s %s", mark, opt)
		} else if st.SingleSelected == i {
			selected = true
		}
		var label string
		if selected {
			label = r.Theme.Success(main)
		} else {
			label = defFG.Render(main)
		}
		line := bullet + " " + num + " " + label
		lines = append(lines, line)
	}
	if customRowVisible(st) {
		i := len(q.Options)
		n := i + 1
		num := formatNum(n)
		var bullet string
		if st.Cursor == i && !st.CustomInputFocused {
			bullet = r.Theme.Primary("●")
		} else {
			bullet = " "
		}
		var label string
		if q.Multiple {
			mark := "[ ]"
			if st.CustomSelected {
				mark = "[x]"
			}
			switch {
			case st.CustomInputFocused:
				var b strings.Builder
				b.WriteString(defFG.Render(mark))
				b.WriteString(" ")
				if st.CustomBuffer != "" {
					b.WriteString(defFG.Render(st.CustomBuffer))
				}
				b.WriteString(r.Theme.Primary("█"))
				label = b.String()
			case st.CustomSelected:
				main := fmt.Sprintf("%s %s", mark, st.CustomBuffer)
				label = r.Theme.Success(main)
			default:
				main := fmt.Sprintf("%s %s", mark, st.CustomBuffer)
				label = defFG.Render(main)
			}
		} else {
			switch {
			case st.CustomInputFocused:
				var b strings.Builder
				if st.CustomBuffer != "" {
					if st.CustomSelected {
						b.WriteString(r.Theme.Success(st.CustomBuffer))
					} else {
						b.WriteString(defFG.Render(st.CustomBuffer))
					}
				}
				b.WriteString(r.Theme.Primary("█"))
				label = b.String()
			case st.CustomSelected:
				label = r.Theme.Success(st.CustomBuffer)
			default:
				if st.CustomBuffer != "" {
					label = defFG.Render(st.CustomBuffer)
				}
			}
		}
		line := bullet + " " + num + " " + label
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// Box wraps content in a themed box.
func (r *ToolRenderer) Box(content string, width int, status ToolStatus) string {
	return r.Theme.Box(content, width, status)
}

// RenderErrorLine returns a themed failure message with a status prefix.
func (r *ToolRenderer) RenderErrorLine(s string) string {
	return "\n " + r.Theme.StatusPrefix(StatusError, "✘") + r.Theme.Error(s)
}
