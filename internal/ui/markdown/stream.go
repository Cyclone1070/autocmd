package markdown

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Stream handles buffering and safe block separation for streaming markdown.
type Stream struct {
	buffer    string
	lastBlock string // Raw markdown of the last successfully flushed block
	parser    goldmark.Markdown
	renderer  Renderer
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

	// Update lastBlock context for next block.
	s.lastBlock = s.extractLastBlockSource(safeContent)

	s.buffer = s.buffer[split:]
	return []string{rendered}, nil
}

// Flush returns any remaining content in the buffer.
func (s *Stream) Flush() ([]string, error) {
	if s.buffer == "" {
		return nil, nil
	}
	rendered, err := s.renderDelta(s.buffer)
	if err != nil {
		return nil, err
	}
	s.buffer = ""
	s.lastBlock = "" // Reset context
	return []string{rendered}, nil
}

// Pending returns the temporary rendered ANSI for the dynamic view.
func (s *Stream) Pending() string {
	if s.buffer == "" {
		return ""
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
		anchor := s.getNodeStart(last, s.buffer)
		return s.scanBack(anchor, s.buffer)
	}

	// Single block: find rightmost \n\n that is not in a sensitive area.
	split := s.getInternalSplit(doc, s.buffer)
	if split > 0 {
		return s.scanBack(split, s.buffer)
	}
	return 0
}

func (s *Stream) scanBack(anchor int, src string) int {
	if anchor <= 0 {
		return 0
	}
	i := anchor - 1
	for i >= 0 {
		if src[i] != '\n' && src[i] != ' ' && src[i] != '\t' && src[i] != '\r' {
			break
		}
		i--
	}
	return i + 1
}

func (s *Stream) getInternalSplit(doc ast.Node, src string) int {
	// Search for the last double-newline gap.
	for i := len(src) - 2; i >= 0; i-- {
		if src[i] == '\n' && src[i+1] == '\n' {
			// Candidates for splitting: the point after the first \n.
			splitPoint := i + 1
			// Skip extra newlines and whitespace to find start of next unit.
			target := splitPoint
			for target < len(src) && (src[target] == '\n' || src[target] == '\r' || src[target] == ' ' || src[target] == '\t') {
				target++
			}
			if target >= len(src) {
				continue
			}

			// Validate if we are in a fenced block.
			if !s.isInsideFencedBlock(doc, target, src) {
				return target
			}
		}
	}
	return 0
}

func (s *Stream) isInsideFencedBlock(node ast.Node, pos int, src string) bool {
	if node.Kind() == ast.KindFencedCodeBlock {
		start := s.getNodeStart(node, src)
		end := s.getNodeEnd(node, src)
		return pos > start && pos < end
	}
	for c := node.FirstChild(); c != nil; c = c.NextSibling() {
		if s.isInsideFencedBlock(c, pos, src) {
			return true
		}
	}
	return false
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
			for i := 0; i < len(sliced); i++ {
				c := sliced[i]
				if c == '-' || c == '*' || c == '_' {
					j := offset + i - 1
					for j >= 0 && src[j] != '\n' && (src[j] == ' ' || src[j] == '\t') {
						j--
					}
					return j + 1
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
		if i >= 0 {
			i--
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

	if stop == 0 {
		if node.Kind() == ast.KindThematicBreak {
			start := s.getNodeStart(node, src)
			if start >= 0 {
				sliced := src[start:]
				idx := strings.IndexByte(sliced, '\n')
				if idx != -1 {
					return start + idx + 1
				}
				return start + len(sliced)
			}
		}
		return 0
	}

	switch node.Kind() {
	case ast.KindFencedCodeBlock:
		sliced := src[stop:]
		for _, fence := range []string{"```", "~~~"} {
			if idx := strings.Index(sliced, fence); idx != -1 {
				lineEnd := strings.IndexByte(sliced[idx:], '\n')
				if lineEnd == -1 {
					return stop + idx + len(fence)
				}
				return stop + idx + lineEnd + 1
			}
		}
	case ast.KindHeading:
		sliced := src[stop:]
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
			t := strings.TrimSpace(underline)
			if (strings.HasPrefix(t, "===") || strings.HasPrefix(t, "---")) && len(t) >= 3 {
				if lineEnd == -1 {
					return stop + offset + len(underline)
				}
				return stop + offset + lineEnd + 1
			}
		}
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

	prefixANSI, err := s.renderer.Render(s.lastBlock)
	if err != nil {
		return "", err
	}

	if strings.HasPrefix(fullANSI, prefixANSI) {
		return fullANSI[len(prefixANSI):], nil
	}
	return fullANSI[len(prefixANSI):], nil
}
