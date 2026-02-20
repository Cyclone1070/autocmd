package markdown

import (
	"fmt"
	"strings"
	"testing"
)

// mockRenderer just returns the input as is
type mockRenderer struct{}

func (m mockRenderer) Render(markdown string) (string, error) {
	return markdown, nil
}

// makeBlock returns a unique block of the given type with an index.
// Content is returned WITHOUT trailing newlines.
func makeBlock(kind string, index int) string {
	switch kind {
	case "PARA_SIMPLE":
		return fmt.Sprintf("Para %d plain text.", index)
	case "PARA_BOLD_END":
		return fmt.Sprintf("Para %d with **bold**.", index)
	case "PARA_ITALIC_END":
		return fmt.Sprintf("Para %d with *italic*.", index)
	case "PARA_CODE_END":
		return fmt.Sprintf("Para %d with `inline code`.", index)
	case "PARA_LINK_END":
		return fmt.Sprintf("Para %d with [link](url).", index)
	case "H1":
		return fmt.Sprintf("# Header One %d", index)
	case "H2":
		return fmt.Sprintf("## Header Two %d", index)
	case "H3":
		return fmt.Sprintf("### Header Three %d", index)
	case "H4":
		return fmt.Sprintf("#### Header Four %d", index)
	case "H5":
		return fmt.Sprintf("##### Header Five %d", index)
	case "SETEXT_1":
		return fmt.Sprintf("Setext One %d\n======", index)
	case "SETEXT_2":
		return fmt.Sprintf("Setext Two %d\n------", index)
	case "LIST_BUL_DAT":
		return fmt.Sprintf("- Item %d A\n- Item %d B", index, index)
	case "LIST_BUL_AST":
		return fmt.Sprintf("* Item %d A\n* Item %d B", index, index)
	case "LIST_NUM_DOT":
		return fmt.Sprintf("1. Item %d A\n2. Item %d B", index, index)
	case "LIST_NUM_PAR":
		return fmt.Sprintf("1) Item %d A\n2) Item %d B", index, index)
	case "CODE_FENCE_GO":
		return fmt.Sprintf("```go\nfunc main%d(){}\n```", index)
	case "CODE_FENCE_TXT":
		return fmt.Sprintf("```text\nplain %d\n```", index)
	case "CODE_FENCE_NONE":
		return fmt.Sprintf("```\nraw %d\n```", index)
	case "CODE_INDENT":
		return fmt.Sprintf("    indented code %d", index)
	case "QUOTE_SIMPLE":
		return fmt.Sprintf("> Quote %d simple.", index)
	case "QUOTE_NESTED":
		return fmt.Sprintf("> > Quote %d nested.", index)
	case "HR_DASH":
		return "---"
	case "HR_STAR":
		return "***"
	case "HR_UNDER":
		return "___"
	default:
		return fmt.Sprintf("Unknown %d", index)
	}
}

func TestStream_Split(t *testing.T) {
	keys := []string{
		"PARA_SIMPLE", "PARA_BOLD_END", "PARA_ITALIC_END", "PARA_CODE_END", "PARA_LINK_END",
		"H1", "H2", "H3", "H4", "H5",
		"SETEXT_1", "SETEXT_2",
		"LIST_BUL_DAT", "LIST_BUL_AST", "LIST_NUM_DOT", "LIST_NUM_PAR",
		"CODE_FENCE_GO", "CODE_FENCE_TXT", "CODE_FENCE_NONE", "CODE_INDENT",
		"QUOTE_SIMPLE", "QUOTE_NESTED",
		"HR_DASH", "HR_STAR", "HR_UNDER",
	}

	runSequence := func(t *testing.T, types []string, newlineCount int) {
		t.Helper()
		s := NewStream(mockRenderer{})
		gap := strings.Repeat("\n", newlineCount)

		var blocks []string
		for i, k := range types {
			blocks = append(blocks, makeBlock(k, i))
		}

		var accFlush strings.Builder

		for i := 0; i < len(blocks); i++ {
			chunk := blocks[i]
			var toAppend string
			if i == 0 {
				toAppend = chunk
			} else {
				toAppend = gap + chunk
			}

			flushed, err := s.Append(toAppend)
			if err != nil {
				t.Fatalf("Step %d Append failed: %v", i, err)
			}
			for _, f := range flushed {
				accFlush.WriteString(f)
			}

			var wantFlush string
			var wantPend string

			if i == 0 {
				wantFlush = ""
				wantPend = blocks[0]
			} else {
				var flushBuilder strings.Builder
				for j := 0; j < i; j++ {
					if j > 0 {
						flushBuilder.WriteString(gap)
					}
					flushBuilder.WriteString(blocks[j])
				}
				wantFlush = flushBuilder.String()
				wantPend = gap + blocks[i]
			}

			gotFlush := accFlush.String()
			gotPend := s.Pending()

			if gotFlush != wantFlush {
				t.Errorf("Seq %v Gap %d Step %d: Flush Mismatch.\nGOT: %q\nWANT: %q", types, newlineCount, i, gotFlush, wantFlush)
			}
			if gotPend != wantPend {
				t.Errorf("Seq %v Gap %d Step %d: Pend Mismatch.\nGOT: %q\nWANT: %q", types, newlineCount, i, gotPend, wantPend)
			}
		}
	}

	newlineGaps := []int{2, 3, 4}

	t.Run("Single", func(t *testing.T) {
		for _, k := range keys {
			runSequence(t, []string{k}, 2)
		}
	})

	t.Run("Pair", func(t *testing.T) {
		for _, gap := range newlineGaps {
			for _, k1 := range keys {
				for _, k2 := range keys {
					runSequence(t, []string{k1, k2}, gap)
				}
			}
		}
	})

	t.Run("Triple", func(t *testing.T) {
		for _, gap := range newlineGaps {
			for _, k1 := range keys {
				for _, k2 := range keys {
					for _, k3 := range keys {
						runSequence(t, []string{k1, k2, k3}, gap)
					}
				}
			}
		}
	})
}
