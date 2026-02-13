package markdown

import (
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Stream handles buffering and safe block separation for streaming markdown.
type Stream struct {
	buffer       string
	safeMarkdown string
	flushedLen   int
	parser       goldmark.Markdown
	renderer     Renderer
}

// NewStream creates a new Stream.
func NewStream(renderer Renderer) *Stream {
	return &Stream{
		parser:   goldmark.New(),
		renderer: renderer,
	}
}

// Append adds a chunk of text, updates the cache, and returns any complete blocks that are safe to flush.
func (s *Stream) Append(chunk string) ([]string, error) {
	s.buffer += chunk

	safePoint, err := s.findSafeSplit()
	if err != nil {
		return nil, nil
	}

	s.safeMarkdown = s.buffer[:safePoint]

	rendered, err := s.render(s.safeMarkdown)
	if err != nil {
		return nil, err
	}

	if len(rendered) > s.flushedLen {
		delta := rendered[s.flushedLen:]
		content := strings.TrimRight(delta, "\n")

		if len(content) == 0 {
			return nil, nil
		}

		s.flushedLen += len(content)
		return []string{content}, nil
	}

	return nil, nil
}

// RenderRemaining returns any remaining content in the buffer as a single block.
func (s *Stream) RenderRemaining() (string, error) {
	fullRendered, err := s.render(s.buffer)
	if err != nil {
		return "", err
	}

	if len(fullRendered) > s.flushedLen {
		delta := fullRendered[s.flushedLen:]
		content := strings.TrimRight(delta, "\n")
		s.flushedLen += len(content)
		return content, nil
	}
	return "", nil
}

// Pending returns the cached rendered content of the buffer (uncertain block).
func (s *Stream) Pending() string {
	fullRendered, _ := s.render(s.buffer)
	if len(fullRendered) > s.flushedLen {
		return strings.TrimRight(fullRendered[s.flushedLen:], "\n")
	}
	return ""
}

func (s *Stream) findSafeSplit() (int, error) {
	src := []byte(s.buffer)
	reader := text.NewReader(src)
	doc := s.parser.Parser().Parse(reader)

	var blocks []ast.Node
	for child := doc.FirstChild(); child != nil; child = child.NextSibling() {
		blocks = append(blocks, child)
	}

	if len(blocks) <= 1 {
		return 0, fmt.Errorf("not enough blocks to flush")
	}

	safeBlock := blocks[len(blocks)-2]

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
		return 0
	}

	safeEnd := getLastStop(safeBlock)
	if safeEnd == 0 {
		return 0, fmt.Errorf("could not find end of safe block")
	}

	if safeBlock.Kind() == ast.KindHeading {
		src := []byte(s.buffer)
		if safeEnd < len(src) && src[safeEnd] == '\n' {
			rest := src[safeEnd+1:]
			lineEnd := 0
			for i, b := range rest {
				if b == '\n' {
					lineEnd = i
					break
				}
				lineEnd = i + 1
			}

			line := rest[:lineEnd]
			t := strings.TrimSpace(string(line))
			if (strings.HasPrefix(t, "===") || strings.HasPrefix(t, "---")) && len(t) >= 3 {
				safeEnd += 1 + lineEnd
			}
		}
	}

	return safeEnd, nil
}

func (s *Stream) render(markdown string) (string, error) {
	if strings.TrimSpace(markdown) == "" {
		return "", nil
	}
	return s.renderer.Render(markdown)
}
