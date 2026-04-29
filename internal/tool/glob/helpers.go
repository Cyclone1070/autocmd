package glob

import (
	"strings"
)

func joinArgs(args []string) string {
	var quoted []string
	for _, arg := range args {
		quoted = append(quoted, quote(arg))
	}
	return strings.Join(quoted, " ")
}

func quote(s string) string {
	if s == "" {
		return "''"
	}
	// POSIX shell quoting: wrap in single quotes and escape any single quotes inside
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
