package grep

import (
	"strings"
)

func joinArgs(args []string) string {
	quoted := make([]string, 0, len(args))
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

func splitGlobs(globStr string) []string {
	var globs []string
	var current strings.Builder
	braceLevel := 0
	for _, char := range globStr {
		switch char {
		case '{':
			braceLevel++
		case '}':
			braceLevel--
		}
		if (char == ' ' || char == ',') && braceLevel == 0 {
			if current.Len() > 0 {
				globs = append(globs, strings.TrimSpace(current.String()))
				current.Reset()
			}
		} else {
			current.WriteRune(char)
		}
	}
	if current.Len() > 0 {
		globs = append(globs, strings.TrimSpace(current.String()))
	}
	return globs
}
