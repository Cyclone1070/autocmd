package ui

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
)

func (m *model) viewDiffDisplay(d domain.DiffDisplay, t *toolState) string {
	sep := separatorStyle.Render(strings.Repeat("─", boxWidth))
	header := d.Header
	if d.Added != 0 || d.Removed != 0 {
		header = fmt.Sprintf("%s (%s, %s)",
			d.Header,
			specialStyle.Render(fmt.Sprintf("+%d", d.Added)),
			errorStyle.Render(fmt.Sprintf("-%d", d.Removed)))
	}

	if t.status == statusError {
		return fmt.Sprintf("%s\n%s\n%s", header, sep, errorStyle.Render(t.err))
	}

	diffContent := d.Diff
	// Simple colorization for diff (green +, red -)
	diffContent = commonDiffColorize(diffContent)

	return fmt.Sprintf("%s\n%s\n%s", header, sep, diffContent)
}

func commonDiffColorize(diff string) string {
	lines := strings.Split(diff, "\n")
	var colored []string
	for _, l := range lines {
		if strings.HasPrefix(l, "+") {
			colored = append(colored, specialStyle.Render(l))
		} else if strings.HasPrefix(l, "-") {
			colored = append(colored, errorStyle.Render(l))
		} else {
			colored = append(colored, l)
		}
	}
	return strings.Join(colored, "\n")
}
