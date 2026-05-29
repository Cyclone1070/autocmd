// Package ui provides shared UI components and utilities for the AutoCmd terminal interface.
package ui

import (
	"regexp"
	"strings"
)

const gapThreshold = 2

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// NormalizeBlock ensures a block has at least one leading newline and no trailing newlines.
// It respects larger gaps by prepending N-1 newlines if the source has more than 2 leading newlines.
// It also trims any visually empty lines (ANSI/whitespace only) from the start and end.
func NormalizeBlock(s string) string {
	lines := strings.Split(s, "\n")
	leading := 0
	for leading < len(lines) && IsVisuallyEmpty(lines[leading]) {
		leading++
	}

	trimmed := TrimVisuallyEmpty(s)
	if trimmed == "" {
		return ""
	}

	// We want to ensure at least one leading newline (matching tea.Printf's trailing one
	// to create a blank line). If the source has more than 2 leading newlines (1+ blank lines),
	// we subtract 1 to account for the tea.Printf addition (or the Join("\n") in history),
	// preserving the extra gap.
	prepend := 1
	if leading > gapThreshold {
		prepend = leading - 1
	}

	return strings.Repeat("\n", prepend) + trimmed
}

// TrimVisuallyEmpty removes leading and trailing lines that contain only
// ANSI escape codes and whitespace.
func TrimVisuallyEmpty(s string) string {
	lines := strings.Split(s, "\n")
	start := 0
	for start < len(lines) && IsVisuallyEmpty(lines[start]) {
		start++
	}
	end := len(lines)
	for end > start && IsVisuallyEmpty(lines[end-1]) {
		end--
	}

	if start >= end {
		return ""
	}
	return strings.Join(lines[start:end], "\n")
}

// IsVisuallyEmpty returns true if the string contains only ANSI escape codes and whitespace.
func IsVisuallyEmpty(s string) bool {
	stripped := ansiRe.ReplaceAllString(s, "")
	return strings.TrimSpace(stripped) == ""
}
