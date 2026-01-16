package ui

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
)

func renderDiff(width int, theme *theme, d domain.DiffDisplay, status toolStatus, err string, prefix string) string {
	sep := theme.separator.Render(strings.Repeat("─", width))
	header := d.Header
	if d.Added != 0 || d.Removed != 0 {
		header = fmt.Sprintf("%s (%s, %s)",
			d.Header,
			theme.successSt.Render(fmt.Sprintf("+%d", d.Added)),
			theme.errorSt.Render(fmt.Sprintf("-%d", d.Removed)))
	}

	if status == statusError {
		return fmt.Sprintf(" %s %s \n%s\n   %s",
			prefix, header, sep, theme.errorSt.Render(err))
	}

	diffContent := commonDiffColorize(d.Diff, theme)
	paddedDiff := pad(diffContent, "") // Just indent, no prefix for diff body

	return fmt.Sprintf(" %s %s \n%s\n%s",
		prefix, header, sep, paddedDiff)
}

func commonDiffColorize(diff string, theme *theme) string {
	lines := strings.Split(diff, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "+") {
			lines[i] = theme.successSt.Render(line)
		} else if strings.HasPrefix(line, "-") {
			lines[i] = theme.errorSt.Render(line)
		}
	}
	return strings.Join(lines, "\n")
}
