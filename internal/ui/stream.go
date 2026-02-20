package ui

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Stream handles buffering and safe block separation for streaming markdown.
type Stream struct {
	buffer        string
	lastBlock     string // Raw markdown of the last successfully flushed block
	lastBlockANSI string // Cached ANSI rendering of lastBlock stripped of doc margins
	lastMargin    string // Trailing newlines stripped from the last render
	parser        goldmark.Markdown
	renderer      Renderer
}

// NewStream creates a new Stream.
func NewStream(renderer Renderer) *Stream {
	return &Stream{
		parser:   goldmark.New(),
		renderer: renderer,
	}
}

// Append adds a chunk of text and returns any complete blocks that are safe to flush.
func (s *Stream) Append(chunk string) ([]string, error) {
	s.buffer += chunk

	split := s.findSafeSplit()
	if split <= 0 {
		return nil, nil
	}

	safeContent := s.buffer[:split]

	// Perform Delta Rendering
	rendered, err := s.renderDelta(safeContent)
	if err != nil {
		return nil, err
	}

	strippedRendered, margin := splitTrailingNewlines(rendered)
	s.lastMargin = margin

	// Update lastBlock context for next block.
	s.lastBlock = s.extractLastBlockSource(safeContent)
	lastANSI, _ := s.renderer.Render(s.lastBlock)
	s.lastBlockANSI, _ = splitTrailingNewlines(lastANSI)

	s.buffer = s.buffer[split:]
	return []string{strippedRendered}, nil
}

// Flush returns any remaining content in the buffer.
func (s *Stream) Flush() ([]string, error) {
	if s.buffer == "" {
		if s.lastMargin != "" {
			m := s.lastMargin
			s.lastMargin = ""
			return []string{m}, nil
		}
		return nil, nil
	}
	rendered, err := s.renderDelta(s.buffer)
	if err != nil {
		return nil, err
	}
	s.buffer = ""
	s.lastBlock = "" // Reset context
	s.lastBlockANSI = ""
	s.lastMargin = ""
	return []string{rendered}, nil
}

// Pending returns the temporary rendered ANSI for the dynamic view.
func (s *Stream) Pending() string {
	if s.buffer == "" {
		return s.lastMargin
	}
	rendered, _ := s.renderDelta(s.buffer)
	return rendered
}

// findSafeSplit identifies the byte offset representing the end of the safe content.
func (s *Stream) findSafeSplit() int {
	reader := text.NewReader([]byte(s.buffer))
	doc := s.parser.Parser().Parse(reader)

	var last ast.Node
	count := 0
	for c := doc.FirstChild(); c != nil; c = c.NextSibling() {
		last = c
		count++
	}

	if count == 0 {
		return 0
	}

	if count >= 2 {
		// Find the second to last node to determine where the previous block ends.
		var prev ast.Node
		curr := doc.FirstChild()
		for curr != nil && curr != last {
			prev = curr
			curr = curr.NextSibling()
		}
		if prev != nil {
			return s.getNodeEnd(prev, s.buffer)
		}
		return 0
	}

	// If we have only 1 node (e.g. 1 giant List or 1 giant CodeBlock), we cannot safely split it
	// without risking broken layout. We must buffer it until it's finished or the stream ends.
	return 0
}

// getNodeStart finds the true byte start of a node including syntax markers.
func (s *Stream) getNodeStart(node ast.Node, src string) int {
	if node == nil {
		return 0
	}

	var anchor int
	if node.Type() == ast.TypeBlock {
		lines := node.Lines()
		if lines.Len() > 0 {
			anchor = lines.At(0).Start
		}
	}

	if anchor == 0 && node.HasChildren() {
		return s.getNodeStart(node.FirstChild(), src)
	}

	if anchor <= 0 {
		if node.Kind() == ast.KindThematicBreak {
			prev := node.PreviousSibling()
			offset := 0
			if prev != nil {
				offset = s.getNodeEnd(prev, src)
			}
			sliced := src[offset:]
			// Thematic break can start with up to 3 spaces indentation.
			// Skip any leading gap newlines/whitespace to find the actual marker.
			for i := 0; i < len(sliced); i++ {
				c := sliced[i]
				if c == '-' || c == '*' || c == '_' {
					// We found the first marker. Backtrack to start of line.
					j := offset + i - 1
					for j >= 0 && src[j] != '\n' {
						j--
					}
					return j + 1
				}
				if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
					// Not a thematic break marker and not whitespace
					break
				}
			}
		}
		return 0
	}

	i := anchor - 1
	for i >= 0 && src[i] != '\n' {
		i--
	}
	startOfLine := i + 1

	if node.Kind() == ast.KindFencedCodeBlock {
		// Fenced code blocks can have an opening fence line before the content lines.
		// Goldmark's anchor points to the first line of CONTENT.
		if i >= 0 {
			i-- // Skip the \n
			for i >= 0 && src[i] != '\n' {
				i--
			}
			startOfLine = i + 1
		}
	}

	return startOfLine
}

// getNodeEnd finds the true byte end of a node including closing syntax.
func (s *Stream) getNodeEnd(node ast.Node, src string) int {
	if node == nil {
		return 0
	}

	var stop int
	if node.Type() == ast.TypeBlock {
		lines := node.Lines()
		if lines.Len() > 0 {
			stop = lines.At(lines.Len() - 1).Stop
		}
	}

	if node.HasChildren() {
		cStop := s.getNodeEnd(node.LastChild(), src)
		if cStop > stop {
			stop = cStop
		}
	}

	switch node.Kind() {
	case ast.KindFencedCodeBlock:
		sliced := src[stop:]
		// Find the closing fence. It must start on a new line.
		idx := strings.IndexAny(sliced, "`~")
		if idx != -1 {
			// Find end of that line
			lineEnd := strings.IndexByte(sliced[idx:], '\n')
			if lineEnd == -1 {
				stop += idx + len(strings.TrimRight(sliced[idx:], " \t\r"))
			} else {
				stop += idx + lineEnd
			}
		}
	case ast.KindHeading:
		sliced := src[stop:]
		// Potential Setext underline
		if len(sliced) > 0 && (sliced[0] == '\n' || sliced[0] == '\r') {
			offset := 1
			if len(sliced) > 1 && sliced[0] == '\r' && sliced[1] == '\n' {
				offset = 2
			}
			rest := sliced[offset:]
			lineEnd := strings.IndexByte(rest, '\n')
			var underline string
			if lineEnd == -1 {
				underline = rest
			} else {
				underline = rest[:lineEnd]
			}
			t := strings.TrimRight(underline, " \t\r")
			if (strings.HasPrefix(t, "===") || strings.HasPrefix(t, "---")) && len(t) >= 3 {
				if lineEnd == -1 {
					stop += offset + len(underline)
				} else {
					stop += offset + lineEnd
				}
			}
		}
	case ast.KindThematicBreak:
		// Thematic break ends at the end of its line
		start := s.getNodeStart(node, src)
		sliced := src[start:]
		idx := strings.IndexByte(sliced, '\n')
		if idx != -1 {
			stop = start + idx
		} else {
			stop = start + len(sliced)
		}
	}

	// Finally, for ALL blocks, ensure we don't include a trailing newline.
	// This ensures our gap separation logic (which expects gaps to start with \n) works.
	for stop > 0 && stop <= len(src) && (src[stop-1] == '\n' || src[stop-1] == '\r') {
		stop--
	}

	return stop
}

// extractLastBlockSource finds the raw markdown of the last top-level block in content.
func (s *Stream) extractLastBlockSource(content string) string {
	reader := text.NewReader([]byte(content))
	doc := s.parser.Parser().Parse(reader)
	last := doc.LastChild()
	if last == nil {
		return ""
	}

	start := s.getNodeStart(last, content)
	end := s.getNodeEnd(last, content)
	if end > len(content) {
		end = len(content)
	}
	if start < 0 {
		start = 0
	}
	return content[start:end]
}

// renderDelta renders newContent with correct spacing relative to lastBlock.
func (s *Stream) renderDelta(newContent string) (string, error) {
	if s.lastBlock == "" {
		return s.renderer.Render(newContent)
	}

	full := s.lastBlock + newContent
	fullANSI, err := s.renderer.Render(full)
	if err != nil {
		return "", err
	}

	if strings.HasPrefix(fullANSI, s.lastBlockANSI) {
		return fullANSI[len(s.lastBlockANSI):], nil
	}

	// Fallback to fresh render if context-aware delta fails (Issue 6)
	return s.renderer.Render(newContent)
}

// splitTrailingNewlines separates the trailing newlines from a string.
func splitTrailingNewlines(text string) (string, string) {
	i := len(text) - 1
	for ; i >= 0; i-- {
		if text[i] != '\n' && text[i] != '\r' {
			break
		}
	}
	return text[:i+1], text[i+1:]
}
