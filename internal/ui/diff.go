package ui

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
)

func (m *model) viewDiffDisplay(d domain.DiffDisplay, t *toolState, prefix string) string {
	sep := m.theme.separator.Render(strings.Repeat("─", m.width-2)) // -2 for borders
	header := d.Header
	if d.Added != 0 || d.Removed != 0 {
		header = fmt.Sprintf("%s (%s, %s)",
			d.Header,
			m.theme.successSt.Render(fmt.Sprintf("+%d", d.Added)),
			m.theme.errorSt.Render(fmt.Sprintf("-%d", d.Removed)))
	}

	if t.status == statusError {
		return fmt.Sprintf(" %s %s \n%s\n   %s",
			prefix, header, sep, m.theme.errorSt.Render(t.err))
	}

	diffContent := m.commonDiffColorize(d.Diff)
	paddedDiff := m.pad(diffContent, "") // Just indent, no prefix for diff body

	return fmt.Sprintf(" %s %s \n%s\n%s",
		prefix, header, sep, paddedDiff)
}

func (m *model) commonDiffColorize(diff string) string {
	lines := strings.Split(diff, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "+") {
			lines[i] = m.theme.successSt.Render(line)
		} else if strings.HasPrefix(line, "-") {
			lines[i] = m.theme.errorSt.Render(line)
		}
	}
	return strings.Join(lines, "\n")
}
