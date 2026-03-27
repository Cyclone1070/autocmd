package summary

import (
	"strings"
)

// Flatten flattens a string to a single line but does NOT truncate.
// This is used for comment fields where we rely on AI instruction for brevity.
func Flatten(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

// Summarize flattens and truncates a string to 100 characters with an explicit indicator.
// This is used for patterns and commands where the front matter is usually most critical.
func Summarize(s string) string {
	const limit = 100
	s = Flatten(s)

	if len(s) <= limit {
		return s
	}

	return s[:limit-3] + "..."
}
