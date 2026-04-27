package prompt

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
)

var ansiStripRE = regexp.MustCompile(`\x1b\[[0-9;]*[mGKH]`)

func stripANSIForTest(s string) string {
	return ansiStripRE.ReplaceAllString(s, "")
}

func simulateStreamOutput(renderer ui.Renderer, chunks []string) string {
	s := NewStream(renderer)
	var out strings.Builder

	for _, ev := range chunks {
		for _, block := range s.Append(ev) {
			// Match tea.Printf line emission semantics
			for line := range strings.SplitSeq(block, "\n") {
				out.WriteString(line)
				out.WriteByte('\n')
			}
		}
	}
	for _, block := range s.Flush() {
		for line := range strings.SplitSeq(block, "\n") {
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}
	return out.String()
}

func canonicalize(s string) string {
	// 1. Split into lines to normalize blank lines and trailing spaces
	lines := strings.Split(s, "\n")
	var resultLines []string
	for _, line := range lines {
		// Only treat as empty if it's just ANSI/spaces (padding junk)
		if ui.IsVisuallyEmpty(line) {
			resultLines = append(resultLines, "")
		} else {
			// Otherwise keep content (including ANSI), only trim trailing plain whitespace
			resultLines = append(resultLines, strings.TrimRight(line, " \t\r"))
		}
	}
	// 2. Join back and trim leading/trailing blank lines from the whole block
	// This ensures we compare the core structure and gaps
	return strings.Trim(strings.Join(resultLines, "\n"), "\n")
}

// mockRenderer just returns the input as is.
type mockRenderer struct{}

func (m mockRenderer) Render(markdown string) string {
	return markdown
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
	case codeIndentKey:
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

const codeIndentKey = "CODE_INDENT"

var (
	testKeys = []string{
		"PARA_SIMPLE", "PARA_BOLD_END", "PARA_ITALIC_END", "PARA_CODE_END", "PARA_LINK_END",
		"H1", "H2", "H3", "H4", "H5",
		"SETEXT_1", "SETEXT_2",
		"LIST_BUL_DAT", "LIST_BUL_AST", "LIST_NUM_DOT", "LIST_NUM_PAR",
		"CODE_FENCE_GO", "CODE_FENCE_TXT", "CODE_FENCE_NONE", codeIndentKey,
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

			flushed := s.Append(toAppend)
			for _, f := range flushed {
				accFlush.WriteString(f)
				accFlush.WriteString("\n") // Simulate tea.Printf behavior
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
				if types[i] == codeIndentKey && (activeListContext != "" || types[i-1] == codeIndentKey) {
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

			gotFlush := canonicalize(accFlush.String())
			wantFlush = canonicalize(wantFlush)
			gotPend := canonicalize(s.Pending())
			wantPend := canonicalize(clipRenderedByCompleteLines(pendingMD.String()))

			if gotFlush != wantFlush {
				t.Errorf("Seq %v Gap %q Step %d: Flush Mismatch.\nGOT: %q\nWANT: %q", types, gap, i, gotFlush, wantFlush)
			}
			if gotPend != wantPend {
				t.Errorf("Seq %v Gap %q Step %d: Pend Mismatch.\nGOT: %q\nWANT: %q", types, gap, i, gotPend, wantPend)
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

	t.Run("CuratedTriples", func(t *testing.T) {
		// Specific triples that are known to be tricky (e.g. nested contexts)
		curated := [][]string{
			{"LIST_BUL_DAT", "LIST_BUL_DAT", "PARA_SIMPLE"},   // Sequential lists
			{"QUOTE_SIMPLE", "QUOTE_NESTED", "PARA_SIMPLE"},   // Nested quotes
			{"CODE_FENCE_GO", "CODE_INDENT", "CODE_FENCE_GO"}, // Code block transitions
			{"PARA_SIMPLE", "HR_DASH", "PARA_SIMPLE"},         // HR separation
			{"H1", "PARA_SIMPLE", "H2"},                       // Header nesting
		}
		for _, gap := range testGaps {
			for _, types := range curated {
				t.Run(fmt.Sprintf("%v/%s", types, gap), func(t *testing.T) {
					t.Parallel()
					runSequence(t, types, gap)
				})
			}
		}
	})
}

func TestStream_InductiveIdentity(t *testing.T) {
	// This test proves that the memory depth of the Stream is exactly 1.
	// We verify that the state after processing [A, B, C] is identical
	// to the state after [X, B, C], regardless of A or X.
	// This ensures that all sequences of length N > 2 are correctly handled
	// if pairs of length 2 are correct.

	for _, gap := range testGaps {
		for _, bType := range testKeys {
			t.Run(fmt.Sprintf("%s/Gap_%q", bType, gap), func(t *testing.T) {
				// Base blocks to ensure history erasure
				aType := "PARA_SIMPLE"
				xType := "CODE_FENCE_GO"
				cType := "PARA_LINK_END" // Fixed third block to trigger flush of bType

				// Handle edge cases where bType is the same as one of our comparison anchors
				if bType == aType {
					aType = "H1"
				}
				if bType == xType {
					xType = "H2"
				}

				s1 := NewStream(mockRenderer{})
				s1.Append(makeBlock(aType, 0) + gap)
				s1.Append(makeBlock(bType, 1) + gap)
				s1.Append(makeBlock(cType, 2))

				s2 := NewStream(mockRenderer{})
				s2.Append(makeBlock(xType, 0) + gap)
				s2.Append(makeBlock(bType, 1) + gap)
				s2.Append(makeBlock(cType, 2))

				// At this point, Block 0 (A or X) and Block 1 (B) have been flushed.
				// lastBlock should be Block 1 (B), and buffer should be Block 2 (C).
				// The state should now be 100% identical.

				if s1.buffer != s2.buffer {
					t.Errorf("Inductive Failure: Buffer mismatch for %s with gap %q: %q vs %q", bType, gap, s1.buffer, s2.buffer)
				}
				if s1.lastBlock != s2.lastBlock {
					t.Errorf("Inductive Failure: LastBlock mismatch for %s with gap %q: %q vs %q", bType, gap, s1.lastBlock, s2.lastBlock)
				}
				if s1.lastBlockANSI != s2.lastBlockANSI {
					t.Errorf("Inductive Failure: LastBlockANSI mismatch for %s with gap %q: %q vs %q", bType, gap, s1.lastBlockANSI, s2.lastBlockANSI)
				}
				if s1.lastMargin != s2.lastMargin {
					t.Errorf("Inductive Failure: LastMargin mismatch for %s with gap %q: %q vs %q", bType, gap, s1.lastMargin, s2.lastMargin)
				}
			})
		}
	}
}

func TestStream_RenderConsistency(t *testing.T) {
	for _, gap := range testGaps {
		t.Run(fmt.Sprintf("Giant_Stream_Gap_%q", gap), func(t *testing.T) {
			t.Parallel()

			renderer := ui.NewGlamourRenderer(80, lipgloss.HasDarkBackground())
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
			wantOut := renderer.Render(fullMD)

			var streamedOut strings.Builder
			for i := 0; i < len(blocks); i++ {
				var toAppend string
				if i == 0 {
					toAppend = blocks[i]
				} else {
					toAppend = gap + blocks[i]
				}
				flushed := s.Append(toAppend)
				for _, f := range flushed {
					streamedOut.WriteString(f)
					streamedOut.WriteString("\n") // Simulate tea.Printf behavior
				}
			}
			finalFlush := s.Flush()
			for _, f := range finalFlush {
				streamedOut.WriteString(f)
				// finalFlush items (like s.lastMargin) are also printed via doFlush/tea.Printf
				streamedOut.WriteString("\n")
			}

			gotOut := canonicalize(streamedOut.String())
			wantOut = canonicalize(wantOut)

			if gotOut != wantOut {
				t.Errorf("Render inconsistency: Output mismatch. GOT %d bytes, WANT %d bytes", len(gotOut), len(wantOut))

				// Find first difference to help debugging
				minLen := min(len(wantOut), len(gotOut))
				for i := range minLen {
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

func TestStream_NoExtraGap_WhenSecondBulletMarkerIsIsolatedChunk(t *testing.T) {
	renderer := ui.NewGlamourRenderer(80, lipgloss.HasDarkBackground())
	markdown := "### 2. The \"Deep Dive\" Bulleted List (Few items, long text)\n\n" +
		"* **The Aurora Borealis (The Northern Lights):** This breathtaking natural phenomenon occurs when charged particles from the sun collide with gaseous atoms in the Earth's atmosphere. These collisions cause the gases to emit light, creating dancing curtains of green, pink, and violet across the high-latitude skies.\n" +
		"* **The Concept of Stoicism:** Originating in ancient Greece, Stoicism is a school of Hellenistic philosophy that teaches the development of self-control and fortitude as a means of overcoming destructive emotions.\n"

	marker := "\n* **The Concept of Stoicism:"
	markerIdx := strings.Index(markdown, marker)
	if markerIdx < 0 {
		t.Fatalf("test fixture marker not found")
	}
	starIdx := markerIdx + 1 // isolate '*' into its own LLM chunk
	chunks := []string{
		markdown[:starIdx],
		"*",
		markdown[starIdx+1:],
	}

	oneShot := stripANSIForTest(renderer.Render(markdown))
	streamed := stripANSIForTest(simulateStreamOutput(renderer, chunks))

	gapPattern := regexp.MustCompile(`(?s)Aurora Borealis.*?\n\s*\n\s*• The Concept of Stoicism`)

	// Guard fixture: one-shot should not contain the gap.
	if gapPattern.MatchString(oneShot) {
		t.Fatalf("invalid fixture: one-shot render already contains extra bullet gap")
	}

	// Regression assertion: streamed output should also not contain the extra gap.
	// Current buggy behavior introduces this gap for certain chunk boundaries.
	if gapPattern.MatchString(streamed) {
		idx := strings.Index(streamed, "The Concept of Stoicism")
		start := max(0, idx-220)
		end := min(len(streamed), idx+120)
		t.Fatalf("streaming regression: isolated bullet marker introduced extra gap between list items; context=%q", streamed[start:end])
	}
}

func TestStream_NoExtraGap_WhenOrderedMarkerIsIsolatedChunk(t *testing.T) {
	renderer := ui.NewGlamourRenderer(80, lipgloss.HasDarkBackground())
	markdown := "1. First stage explains context in detail.\n" +
		"2. Second stage elaborates on scaling and integration.\n"

	// Isolate the start of the second marker as its own streamed event.
	marker := "\n2. "
	markerIdx := strings.Index(markdown, marker)
	if markerIdx < 0 {
		t.Fatalf("ordered marker fixture not found")
	}
	digitIdx := markerIdx + 1 // points at '2'
	chunks := []string{
		markdown[:digitIdx],
		"2",
		markdown[digitIdx+1:],
	}

	oneShot := stripANSIForTest(renderer.Render(markdown))
	streamed := stripANSIForTest(simulateStreamOutput(renderer, chunks))
	gapPattern := regexp.MustCompile(`(?s)First stage.*?\n\s*\n\s*2\. Second stage`)

	if gapPattern.MatchString(oneShot) {
		t.Fatalf("invalid fixture: one-shot ordered list already contains extra gap")
	}
	if gapPattern.MatchString(streamed) {
		t.Fatalf("streaming regression: isolated ordered marker introduced extra gap between items")
	}
}

func TestStream_StillSplitsParagraphBeforeHeading(t *testing.T) {
	// Control case: non-list boundaries should keep existing split behavior.
	s := NewStream(mockRenderer{})
	flushed := s.Append("para\n\n## heading")
	if len(flushed) == 0 {
		t.Fatalf("expected paragraph to flush before heading boundary")
	}
}

func TestStream_Pending_HidesTrailingIncompleteLine(t *testing.T) {
	s := NewStream(mockRenderer{})
	s.Append("abcdefghij")
	assert.Equal(t, "", s.Pending())
}

func TestClipRenderedByCompleteLines_ShowsOnlyCompleteLines(t *testing.T) {
	assert.Equal(t, "line one", clipRenderedByCompleteLines("line one\nline two partial"))
}

func TestClipRenderedByCompleteLines_ShowsAllWhenEndsWithNewline(t *testing.T) {
	assert.Equal(t, "line one\nline two\n", clipRenderedByCompleteLines("line one\nline two\n"))
}
