package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// streamingMarkdown handles buffering and safe block separation for streaming content.
type streamingMarkdown struct {
	buffer       string
	safeMarkdown string // Markdown up to safe split point
	flushedLen   int    // Rendered bytes already flushed
	parser       goldmark.Markdown
	glamour      *glamour.TermRenderer
}

// newStreamingMarkdown creates a new instance.
func newStreamingMarkdown(renderer *glamour.TermRenderer) *streamingMarkdown {
	return &streamingMarkdown{
		parser:  goldmark.New(),
		glamour: renderer,
	}
}

// append adds a chunk of text, updates the cache, and returns any complete blocks that are safe to flush.
// The last incomplete block is kept in the buffer.
func (s *streamingMarkdown) append(chunk string) ([]string, error) {
	s.buffer += chunk

	// Find safe split using Goldmark
	safePoint, err := s.findSafeSplit()
	if err != nil {
		// If error (e.g. not enough blocks), treat as nothing safe to flush
		return nil, nil
	}

	// Update the safe portion of the buffer
	s.safeMarkdown = s.buffer[:safePoint]

	// Re-render the safe portion (prefix-consistent)
	rendered, err := s.render(s.safeMarkdown)
	if err != nil {
		return nil, err
	}

	// Calculate and flush the delta (new content only)
	if len(rendered) > s.flushedLen {
		delta := rendered[s.flushedLen:]

		// [FIX] Treat trailing newlines as unstable
		// 1. Trim ALL trailing newlines (they might be collapsed later by glamour)
		content := strings.TrimRight(delta, "\n")

		// 2. If content is empty (offsets were just newlines), WAIT.
		//    Do NOT flush empty strings (which become \n via Println)
		//    This prevents "Phantom Newlines" where Partial render has more newlines than Full render.
		if len(content) == 0 {
			return nil, nil // Wait for more content or RenderRemaining
		}

		// 3. Update flushedLen: exactly what we consumed (content length).
		//    We do NOT add +1 because we use tea.Print (no implicit newline).
		s.flushedLen += len(content)
		return []string{content}, nil
	}

	return nil, nil
}

// RenderRemaining returns any remaining content in the buffer as a single block.
func (s *streamingMarkdown) RenderRemaining() (string, error) {
	// Render the FULL buffer
	fullRendered, err := s.render(s.buffer)
	if err != nil {
		return "", err
	}

	// Flush whatever hasn't been flushed yet
	if len(fullRendered) > s.flushedLen {
		delta := fullRendered[s.flushedLen:]

		content := strings.TrimRight(delta, "\n")

		// Always flush at the end
		// We update flushedLen for completeness, though stream ends here
		s.flushedLen += len(content)

		return content, nil
	}
	return "", nil
}

// pending returns the cached rendered content of the buffer (uncertain block).
// Called by View() on every render cycle. INFALLIBLE.
func (s *streamingMarkdown) pending() string {
	// Render the FULL buffer
	// Note: We re-render full buffer here because View needs to show
	// both flushed AND unflushed content combined.
	// However, we only need to return the UNFLUSHED portion for display?
	// No, View() usually stitches things together.
	// Wait, standard UI model appends flushed content to history view, and shows current pending in active view.
	// So pending() needs to return: render(full) - render(safe)
	// OR essentially: render(full)[flushedLen:]

	fullRendered, _ := s.render(s.buffer)
	if len(fullRendered) > s.flushedLen {
		return strings.TrimRight(fullRendered[s.flushedLen:], "\n")
	}
	return ""
}

// findSafeSplit determines the index to split the buffer for safe flushing.
// It returns the end index of the second-to-last block.
func (s *streamingMarkdown) findSafeSplit() (int, error) {
	src := []byte(s.buffer)
	reader := text.NewReader(src)
	doc := s.parser.Parser().Parse(reader)

	// Collect top-level blocks
	var blocks []ast.Node
	for child := doc.FirstChild(); child != nil; child = child.NextSibling() {
		blocks = append(blocks, child)
	}

	if len(blocks) <= 1 {
		return 0, fmt.Errorf("not enough blocks to flush")
	}

	// The last block is unsafe (growing). Everything before it is safe.
	// Safe split point is the end of the second-to-last block.
	safeBlock := blocks[len(blocks)-2]

	// Helper to find the LAST content end of a node (recursive)
	var getLastStop func(node ast.Node) int
	getLastStop = func(node ast.Node) int {
		lines := node.Lines()
		if lines.Len() > 0 {
			lastLine := lines.At(lines.Len() - 1)
			return lastLine.Stop
		}
		if node.HasChildren() {
			var lastChild ast.Node
			for c := node.FirstChild(); c != nil; c = c.NextSibling() {
				lastChild = c
			}
			if lastChild != nil {
				return getLastStop(lastChild)
			}
		}
		// Fallback for nodes without lines (rare/impossible for standard blocks)
		return 0
	}

	safeEnd := getLastStop(safeBlock)
	if safeEnd == 0 {
		return 0, fmt.Errorf("could not find end of safe block")
	}

	// [FIX] Setext Headings have markup (=== or ---) AFTER the content.
	// getLastStop only gives us the end of the text content, so we would miss the underline.
	// This causes "Heading" to render as "Paragraph", breaking prefix stability.
	// We scan forward to include the Setext underline if present.
	if safeBlock.Kind() == ast.KindHeading {
		src := []byte(s.buffer)
		// Check for newline followed by at least 3 = or -
		if safeEnd < len(src) && src[safeEnd] == '\n' {
			// Check next line
			rest := src[safeEnd+1:]
			// Find length of next line
			lineEnd := 0
			for i, b := range rest {
				if b == '\n' {
					lineEnd = i
					break
				}
				lineEnd = i + 1
			}

			line := rest[:lineEnd]
			trimmed := string(line) // TrimSpace? Setext allows indentation?
			// Goldmark spec: up to 3 spaces indentation.
			// Simpler: Check if line consists of ONLY = or - (plus whitespace).
			// And at least 3 chars.

			// Actually, just checking startsWith === or --- is good heuristic for now.
			// Goldmark handles parsing, we just need to extend the range.
			t := strings.TrimSpace(trimmed)
			if (strings.HasPrefix(t, "===") || strings.HasPrefix(t, "---")) && len(t) >= 3 {
				// It's likely a Setext underline. Include it.
				safeEnd += 1 + lineEnd
			}
		}
	}

	// We don't need complex gap analysis for Fenced Code because Open Code Blocks
	// are prefix-compatible with Closed Code Blocks (verified).
	// So we leave safeEnd as-is for other types.

	return safeEnd, nil
}

func (s *streamingMarkdown) render(markdown string) (string, error) {
	if strings.TrimSpace(markdown) == "" {
		return "", nil
	}
	return s.glamour.Render(markdown)
}
