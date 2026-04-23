// Package tool provides rendering for tool outputs (StringDisplay, DiffDisplay, BashDisplay).
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
	Gate(lines []string) ([]string, int)
}

const (
	ToolInsetPrefix              = "    "
	ToolFirstContentGutterPrefix = "   ⎿ "
	ToolContentGutterPrefix      = "     "
)

// ToolBlockSpec is the semantic rendering contract passed from ToolRenderer to Theme.
// Theme consumes this structure for placement and styling only.
type ToolBlockSpec struct {
	HeaderLines  []string
	ContentLines []string
	Status       ToolStatus
	Frame        string
}

type ContentTruncateMode int

const (
	TruncateNone ContentTruncateMode = iota
	TruncateTailKeepLatest
)

type RenderSpecOptions struct {
	TruncateMode             ContentTruncateMode
	TruncateFromContentIndex int
}

// ToolRenderer provides rendering for tool outputs (StringDisplay, DiffDisplay, BashDisplay).
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

func (r *ToolRenderer) SetShortToolBlock(b bool) {
	r.Theme.ShortToolBlock = b
}

func (r *ToolRenderer) formatError(prefix string, err string) string {
	if prefix == "" {
		return r.Theme.Error(err)
	}
	return r.Theme.Error(prefix + " - " + err)
}

func (r *ToolRenderer) mutedLines(lines []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = r.Theme.Muted(line)
	}
	return out
}

func (r *ToolRenderer) statusColor(status ToolStatus, s string) string {
	switch status {
	case StatusRunning:
		return r.Theme.Primary(s)
	case StatusSuccess:
		return r.Theme.Success(s)
	case StatusError:
		return r.Theme.Error(s)
	default:
		return s
	}
}

func (r *ToolRenderer) wrapLines(lines []string, firstWidth, continuationWidth int) []string {
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
		parts := strings.SplitSeq(wrapped, "\n")
		for part := range parts {
			out = append(out, strings.TrimRight(part, " "))
		}
	}
	return out
}

// RenderString renders StringDisplay.
func (r *ToolRenderer) RenderString(d domain.StringDisplay, status ToolStatus, err string, frame string) string {
	spec, ok := r.buildStringSpec(d, status, err, frame)
	if !ok {
		return ""
	}
	return r.renderSpec(spec, RenderSpecOptions{TruncateMode: TruncateNone, TruncateFromContentIndex: 0})
}

func (r *ToolRenderer) buildStringSpec(d domain.StringDisplay, status ToolStatus, err string, frame string) (ToolBlockSpec, bool) {
	spec := ToolBlockSpec{
		Status: status,
		Frame:  frame,
	}

	header := d.Description
	if status == StatusError && err != "" {
		header = r.formatError(header, err)
	}
	if header != "" {
		if status == StatusError {
			spec.HeaderLines = []string{header}
		} else {
			spec.HeaderLines = []string{r.statusColor(status, header)}
		}
	}
	if d.Content != "" {
		spec.ContentLines = r.mutedLines(strings.Split(d.Content, "\n"))
	}

	if len(spec.HeaderLines) == 0 && len(spec.ContentLines) == 0 {
		return ToolBlockSpec{}, false
	}
	return spec, true
}

// RenderDiff renders DiffDisplay.
func (r *ToolRenderer) RenderDiff(d domain.DiffDisplay, status ToolStatus, err string, frame string) string {
	spec, ok := r.buildDiffSpec(d, status, err, frame)
	if !ok {
		return ""
	}
	return r.renderSpec(spec, RenderSpecOptions{TruncateMode: TruncateTailKeepLatest, TruncateFromContentIndex: 1})
}

func (r *ToolRenderer) buildDiffSpec(d domain.DiffDisplay, status ToolStatus, err string, frame string) (ToolBlockSpec, bool) {
	header := d.Description
	target := d.Target
	spec := ToolBlockSpec{
		HeaderLines: []string{r.statusColor(status, header)},
		Status:      status,
		Frame:       frame,
	}

	if status == StatusError {
		spec.HeaderLines[0] = r.formatError(header, err)
		if target != "" {
			spec.ContentLines = append(spec.ContentLines, r.Theme.Muted(target))
		}
		return spec, true
	}

	// Add stats to target if success
	if status == StatusSuccess && (d.Added != 0 || d.Removed != 0) {
		target = fmt.Sprintf("%s (%s, %s)",
			target,
			r.Theme.Success(fmt.Sprintf("+%d", d.Added)),
			r.Theme.Error(fmt.Sprintf("-%d", d.Removed)))
	}

	diffContent := r.colorizeDiff(d.Diff)

	if target != "" {
		spec.ContentLines = append(spec.ContentLines, r.Theme.Muted(target))
	}
	if diffContent != "" && !r.Theme.ShortToolBlock {
		spec.ContentLines = append(spec.ContentLines, strings.Split(diffContent, "\n")...)
	}
	return spec, true
}

func (r *ToolRenderer) colorizeDiff(diff string) string {
	lines := strings.Split(diff, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "+") {
			lines[i] = r.Theme.Success(line)
		} else if strings.HasPrefix(line, "-") {
			lines[i] = r.Theme.Error(line)
		} else {
			lines[i] = r.Theme.Muted(line)
		}
	}
	return strings.Join(lines, "\n")
}

// RenderBash renders BashDisplay.
func (r *ToolRenderer) RenderBash(d domain.BashDisplay, output string, status ToolStatus, err string, frame string) string {
	spec, ok := r.buildBashSpec(d, output, status, err, frame)
	if !ok {
		return ""
	}
	return r.renderSpec(spec, RenderSpecOptions{TruncateMode: TruncateTailKeepLatest, TruncateFromContentIndex: 1})
}

func (r *ToolRenderer) buildBashSpec(d domain.BashDisplay, output string, status ToolStatus, err string, frame string) (ToolBlockSpec, bool) {
	header := d.Description
	spec := ToolBlockSpec{
		HeaderLines: []string{r.statusColor(status, header)},
		Status:      status,
		Frame:       frame,
	}
	if status == StatusError {
		spec.HeaderLines[0] = r.formatError(header, err)
		spec.ContentLines = []string{r.Theme.Muted(fmt.Sprintf("%s$ %s", d.Cwd, d.Command))}
		return spec, true
	}
	bashOutput := strings.TrimRight(output, "\n")

	spec.ContentLines = append(spec.ContentLines, r.Theme.Muted(fmt.Sprintf("%s$ %s", d.Cwd, d.Command)))
	if bashOutput != "" && !r.Theme.ShortToolBlock {
		spec.ContentLines = append(spec.ContentLines, r.mutedLines(strings.Split(bashOutput, "\n"))...)
	}
	return spec, true
}

// RenderQuestion renders QuestionDisplay with interactive state (cursor, toggles, custom text).
// Unlike other tool renderers, it never prepends a status prefix (spinner/checkmark); the question header is the first line.
func (r *ToolRenderer) RenderQuestion(d domain.QuestionDisplay, state QuestionUIState, status ToolStatus, err string, frame string) string {
	spec, ok := r.buildQuestionSpec(d, state, status, err, frame)
	if !ok {
		return ""
	}
	return r.renderSpec(spec, RenderSpecOptions{TruncateMode: TruncateNone, TruncateFromContentIndex: 0})
}

func (r *ToolRenderer) buildQuestionSpec(d domain.QuestionDisplay, state QuestionUIState, status ToolStatus, err string, frame string) (ToolBlockSpec, bool) {
	if len(d.Questions) == 0 {
		return ToolBlockSpec{}, false
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
			summary = r.formatError(plainSummary, err)
		} else if answered == n {
			summary = r.Theme.Success("All questions answered")
		} else {
			summary = r.Theme.Error(plainSummary)
		}
		body := summary + "\n\n" + r.renderQuestionReviewBlock(stRender)
		body += "\n\n" + r.renderQuestionFooter()
		lines := strings.Split(body, "\n")
		spec := ToolBlockSpec{
			Status: status,
			Frame:  frame,
		}
		spec.HeaderLines = []string{lines[0]}
		if len(lines) > 1 {
			spec.ContentLines = lines[1:]
		}
		return spec, true
	}

	cur := active + 1
	q := d.Questions[active]
	st := stRender.Per[active]

	baseHeader := fmt.Sprintf("Question %d/%d", cur, n)
	header := r.Theme.Primary(baseHeader)
	if status == StatusError && err != "" {
		header = r.formatError(baseHeader, err)
	}

	parts := []string{header}
	parts = append(parts, q.Question)
	parts = append(parts, r.renderQuestionOptionBlock(q, st))

	body := strings.Join(parts, "\n\n")
	body += "\n\n" + r.renderQuestionFooter()
	lines := strings.Split(body, "\n")
	spec := ToolBlockSpec{
		Status:      status,
		Frame:       frame,
		HeaderLines: []string{lines[0]},
	}
	if len(lines) > 1 {
		spec.ContentLines = lines[1:]
	}
	return spec, true
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
// the same continuation content width as tool block wrapping so footer/separator
// lines don't overflow and wrap unexpectedly.
func (r *ToolRenderer) questionInnerWidth() int {
	inner := r.Width - lipgloss.Width(ToolInsetPrefix+ToolContentGutterPrefix)
	if inner < 12 {
		return 12
	}
	return inner
}

func (r *ToolRenderer) renderQuestionFooter() string {
	inner := r.questionInnerWidth()

	// Key (primary) + label (muted), with gaps between groups like the toolbox hint row.
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(r.Theme.PrimaryColor())
	keyLabel := func(key, label string) string {
		return keyStyle.Render(key) + " " + r.Theme.Muted(label)
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
	return row
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
		if q.MultiSelect {
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
		if q.MultiSelect {
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

func (r *ToolRenderer) renderSpec(spec ToolBlockSpec, opts RenderSpecOptions) string {
	headerPrefixWidth := lipgloss.Width(r.Theme.StatusPrefix(spec.Status, spec.Frame))
	headerContinuationWidth := r.Width - lipgloss.Width(ToolInsetPrefix)
	headerFirstWidth := headerContinuationWidth - headerPrefixWidth

	contentFirstWidth := r.Width - lipgloss.Width(ToolInsetPrefix+ToolFirstContentGutterPrefix)
	contentContinuationWidth := r.Width - lipgloss.Width(ToolInsetPrefix+ToolContentGutterPrefix)

	spec.HeaderLines = r.wrapLines(spec.HeaderLines, headerFirstWidth, headerContinuationWidth)
	if opts.TruncateMode == TruncateTailKeepLatest && len(spec.ContentLines) > 0 {
		start := min(max(opts.TruncateFromContentIndex, 0), len(spec.ContentLines))

		// Preserve/truncate boundary is defined on logical lines first.
		preservedLogical := spec.ContentLines[:start]
		truncatableLogical := spec.ContentLines[start:]

		preservedWrapped := r.wrapLines(preservedLogical, contentFirstWidth, contentContinuationWidth)
		if len(truncatableLogical) == 0 {
			spec.ContentLines = preservedWrapped
		} else {
			truncFirstWidth := contentContinuationWidth
			if len(preservedLogical) == 0 {
				truncFirstWidth = contentFirstWidth
			}
			truncWrapped := r.wrapLines(truncatableLogical, truncFirstWidth, contentContinuationWidth)
			gatedLines, indicatorLines := r.gater.Gate(truncWrapped)
			for i := 0; i < indicatorLines && i < len(gatedLines); i++ {
				gatedLines[i] = r.Theme.Muted(gatedLines[i])
			}
			spec.ContentLines = append(preservedWrapped, gatedLines...)
		}
	} else {
		spec.ContentLines = r.wrapLines(spec.ContentLines, contentFirstWidth, contentContinuationWidth)
	}

	return r.Theme.RenderToolBlock(spec)
}
