package ui

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
)

func renderDiff(width int, theme *theme, d domain.DiffDisplay, status toolStatus, err string, prefix string) string {
	header := d.Header
	if d.Added != 0 || d.Removed != 0 {
		header = fmt.Sprintf("%s (%s, %s)",
			d.Header,
			theme.Success(fmt.Sprintf("+%d", d.Added)),
			theme.Error(fmt.Sprintf("-%d", d.Removed)))
	}

	if status == statusError {
		header = formatError(header, err, theme)
		return fmt.Sprintf(" %s %s ", prefix, header)
	}

	diffContent := commonDiffColorize(d.Diff, theme)
	paddedDiff := pad(diffContent, "") // Just indent, no prefix for diff body
	sep := theme.Separator(width, status)

	return fmt.Sprintf(" %s %s \n%s\n%s",
		prefix, header, sep, paddedDiff)
}

func commonDiffColorize(diff string, theme *theme) string {
	lines := strings.Split(diff, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "+") {
			lines[i] = theme.Success(line)
		} else if strings.HasPrefix(line, "-") {
			lines[i] = theme.Error(line)
		}
	}
	return strings.Join(lines, "\n")
}
