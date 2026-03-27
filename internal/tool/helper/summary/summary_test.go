package summary

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSummarize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Short string",
			input:    "Hello world",
			expected: "Hello world",
		},
		{
			name:     "Multi-line flattening",
			input:    "Line 1\nLine 2",
			expected: "Line 1 Line 2",
		},
		{
			name:     "Exactly 100 chars",
			input:    strings.Repeat("a", 100),
			expected: strings.Repeat("a", 100),
		},
		{
			name:     "101 chars (Head-only)",
			input:    strings.Repeat("H", 90) + "M" + strings.Repeat("T", 10),
			expected: strings.Repeat("H", 90) + "MTTTTTT...",
		},
		{
			name:     "Long string with newlines",
			input:    strings.Repeat("L\n", 60), // 120 chars total flattened
			expected: strings.Repeat("L ", 48) + "L...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Summarize(tt.input)
			assert.Equal(t, tt.expected, result)
			assert.LessOrEqual(t, len(result), 100)
		})
	}
}
