package loop

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/ui"
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
	case "CODE_FENCE_TILDE_4":
		return fmt.Sprintf("~~~~go\nfunc main%d(){}\n~~~~", index)
	case "HR_SPACED_DASH":
		return "- - -"
	case "HR_SPACED_STAR":
		return "*   *   *"
	case "SETEXT_PADDED":
		return fmt.Sprintf("Setext Padded %d\n===   ", index)
	default:
		return fmt.Sprintf("Unknown %d", index)
	}
}

var (
	testKeys = []string{
		"PARA_SIMPLE", "PARA_BOLD_END", "PARA_ITALIC_END", "PARA_CODE_END", "PARA_LINK_END",
		"H1", "H2", "H3", "H4", "H5",
		"SETEXT_1", "SETEXT_2",
		"LIST_BUL_DAT", "LIST_BUL_AST", "LIST_NUM_DOT", "LIST_NUM_PAR",
		"CODE_FENCE_GO", "CODE_FENCE_TXT", "CODE_FENCE_NONE", "CODE_INDENT",
		"QUOTE_SIMPLE", "QUOTE_NESTED",
		"HR_DASH", "HR_STAR", "HR_UNDER",
		"CODE_FENCE_TILDE_4", "HR_SPACED_DASH", "HR_SPACED_STAR", "SETEXT_PADDED",
	}
	testGaps = []string{"\n\n", "\n\n\n", "\r\n\r\n", "\n   \n", "\r\n  \r\n"}
)

func TestStream_Split(t *testing.T) {
	runSequence := func(t *testing.T, types []string, gap string) {
		t.Helper()
		s := NewStream(mockRenderer{})

		var blocks []string
		for i, k := range types {
			blocks = append(blocks, makeBlock(k, i))
		}

		var accFlush strings.Builder
		var pendingMD strings.Builder
		var activeListContext string

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

			// Track active list context to simulate Goldmark's AST grouping correctly across multiple blocks.
			if i == 0 {
				pendingMD.WriteString(chunk)
				if strings.HasPrefix(types[0], "LIST_") {
					activeListContext = types[0]
				}
			} else {
				// Determine if Goldmark would split types[i] from the currently pending block
				split := true
				if types[i] == "CODE_INDENT" && (activeListContext != "" || types[i-1] == "CODE_INDENT") {
					split = false
				} else if strings.HasPrefix(types[i], "LIST_") && activeListContext == types[i] {
					split = false
				}

				if split {
					// We expected the PREVIOUS pending content to be flushed now.
					// And the NEW chunk to become the new pending.
					pendingMD.Reset()
					pendingMD.WriteString(gap + chunk)
					if strings.HasPrefix(types[i], "LIST_") {
						activeListContext = types[i]
					} else {
						activeListContext = ""
					}
				} else {
					// Sticky! It shouldn't have flushed anything extra.
					pendingMD.WriteString(gap + chunk)
				}
			}

			// The total accumulated flush should be everything except what is currently pending.
			fullMD := strings.Join(blocks[:i+1], gap)
			wantFlush := ""
			if len(fullMD) > len(pendingMD.String()) {
				wantFlush = fullMD[:len(fullMD)-len(pendingMD.String())]
			}
			// Trim trailing newlines because Stream flushes strip them
			wantFlush = strings.TrimRight(wantFlush, "\n\r")

			gotFlush := strings.TrimRight(accFlush.String(), "\n\r")
			gotPend := s.Pending()

			if gotFlush != wantFlush {
				t.Errorf("Seq %v Gap %q Step %d: Flush Mismatch.\nGOT: %q\nWANT: %q", types, gap, i, gotFlush, wantFlush)
			}
			if gotPend != pendingMD.String() {
				t.Errorf("Seq %v Gap %q Step %d: Pend Mismatch.\nGOT: %q\nWANT: %q", types, gap, i, gotPend, pendingMD.String())
			}
		}
	}

	t.Run("Single", func(t *testing.T) {
		for _, k := range testKeys {
			runSequence(t, []string{k}, "\n\n")
		}
	})

	t.Run("Pair", func(t *testing.T) {
		for _, gap := range testGaps {
			for _, k1 := range testKeys {
				for _, k2 := range testKeys {
					types := []string{k1, k2}
					t.Run(fmt.Sprintf("%v/%s", types, gap), func(t *testing.T) {
						t.Parallel()
						runSequence(t, types, gap)
					})
				}
			}
		}
	})

	t.Run("Triple", func(t *testing.T) {
		for _, gap := range testGaps {
			for _, k1 := range testKeys {
				for _, k2 := range testKeys {
					for _, k3 := range testKeys {
						types := []string{k1, k2, k3}
						t.Run(fmt.Sprintf("%v/%s", types, gap), func(t *testing.T) {
							t.Parallel()
							runSequence(t, types, gap)
						})
					}
				}
			}
		}
	})
}

func TestStream_RenderConsistency(t *testing.T) {
	for _, gap := range testGaps {
		t.Run(fmt.Sprintf("Giant_Stream_Gap_%q", gap), func(t *testing.T) {
			t.Parallel()

			renderer, err := ui.NewGlamourRenderer(80)
			if err != nil {
				t.Fatalf("Failed to create renderer: %v", err)
			}
			s := NewStream(renderer)

			var types []string
			for _, k1 := range testKeys {
				for _, k2 := range testKeys {
					types = append(types, k1, k2)
				}
			}

			var blocks []string
			for i, k := range types {
				blocks = append(blocks, makeBlock(k, i))
			}

			fullMD := strings.Join(blocks, gap)
			wantOut, err := renderer.Render(fullMD)
			if err != nil {
				t.Fatalf("Render failed: %v", err)
			}

			var streamedOut strings.Builder
			for i := 0; i < len(blocks); i++ {
				var toAppend string
				if i == 0 {
					toAppend = blocks[i]
				} else {
					toAppend = gap + blocks[i]
				}
				flushed, err := s.Append(toAppend)
				if err != nil {
					t.Fatalf("Append failed: %v", err)
				}
				for _, f := range flushed {
					streamedOut.WriteString(f)
				}
			}
			finalFlush, err := s.Flush()
			if err != nil {
				t.Fatalf("Flush failed: %v", err)
			}
			for _, f := range finalFlush {
				streamedOut.WriteString(f)
			}

			gotOut := streamedOut.String()
			if gotOut != wantOut {
				t.Errorf("Render inconsistency: Output mismatch. GOT %d bytes, WANT %d bytes", len(gotOut), len(wantOut))

				// Find first difference to help debugging
				minLen := min(len(wantOut), len(gotOut))
				for i := 0; i < minLen; i++ {
					if gotOut[i] != wantOut[i] {
						start := max(i-50, 0)

						endGot := min(i+50, len(gotOut))

						endWant := min(i+50, len(wantOut))

						t.Logf("First mismatch at byte %d:\nGOT  context: %q\nWANT context: %q",
							i, gotOut[start:endGot], wantOut[start:endWant])
						break
					}
				}
			}
		})
	}
}
