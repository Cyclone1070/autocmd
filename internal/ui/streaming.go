package ui

import (
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// streamingMarkdown handles buffering and safe block separation for streaming content.
type streamingMarkdown struct {
	buffer          string
	pendingRendered string // Cached render of the buffer
	parser          goldmark.Markdown
	glamour         *glamour.TermRenderer
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
	return s.process(false)
}

// flush returns any remaining content in the buffer as a single block.
func (s *streamingMarkdown) flush() (string, error) {
	flushed, err := s.process(true)
	if err != nil {
		return "", err
	}
	// If there's anything left (the last block), return it
	if s.buffer != "" {
		out, err := s.render(s.buffer)
		s.buffer = ""
		s.pendingRendered = "" // Clear cache
		return out, err
	}
	if len(flushed) > 0 {
		return strings.Join(flushed, "\n"), nil
	}
	return "", nil
}

// pending returns the cached rendered content of the buffer (uncertain block).
// Called by View() on every render cycle. INFALLIBLE.
func (s *streamingMarkdown) pending() string {
	return strings.TrimRight(s.pendingRendered, "\n")
}

func (s *streamingMarkdown) process(forceFlush bool) ([]string, error) {
	if s.buffer == "" {
		s.pendingRendered = ""
		return nil, nil
	}

	src := []byte(s.buffer)
	reader := text.NewReader(src)
	doc := s.parser.Parser().Parse(reader)

	// Collect top-level blocks
	var blocks []ast.Node
	for child := doc.FirstChild(); child != nil; child = child.NextSibling() {
		blocks = append(blocks, child)
	}

	if len(blocks) == 0 {
		out, err := s.render(s.buffer)
		s.pendingRendered = out
		return nil, err
	}

	// We only flush blocks if we have more than one (meaning earlier ones are closed),
	// OR if forceFlush is true.
	if len(blocks) <= 1 && !forceFlush {
		out, err := s.render(s.buffer)
		s.pendingRendered = out
		return nil, err
	}

	// Determine where to split the buffer
	splitIndex := 0

	if forceFlush {
		splitIndex = len(src)
	} else {
		// Flush up to the start of the last block
		// By default, the last top-level block is considered "pending" and unsafe to flush.
		pendingBlock := blocks[len(blocks)-1]

		// However, if the pending block is a container (List or Blockquote),
		// we might be able to flush *part* of it (e.g. the first N-1 items of a list).
		// We drill down to find the most specific "last child" that is truly pending.
		for {
			isContainer := pendingBlock.Kind() == ast.KindList || pendingBlock.Kind() == ast.KindBlockquote
			if !isContainer {
				break
			}

			// Check children
			var lastChild ast.Node
			childCount := 0
			for c := pendingBlock.FirstChild(); c != nil; c = c.NextSibling() {
				lastChild = c
				childCount++
			}

			// If we have multiple children, the earlier ones are safe to flush.
			// We target the *last* child as the new pending point.
			if childCount > 1 && lastChild != nil {
				pendingBlock = lastChild
				continue
			}
			break
		}

		// If pending block exists and has lines, find its TRUE start (including syntax markers)
		if pendingBlock.Lines().Len() > 0 {
			contentStart := pendingBlock.Lines().At(0).Start
			splitIndex = adjustBlockStart(src, pendingBlock, contentStart)
		} else {
			// Weird case (empty block?), safer to not flush anything if we can't find start
			out, err := s.render(s.buffer)
			s.pendingRendered = out
			return nil, err
		}
	}

	// Content to flush is src[:splitIndex]
	toFlush := string(src[:splitIndex])

	// Update buffer to keep only the remainder
	s.buffer = string(src[splitIndex:])

	// Update cached render for the remainder
	var cacheErr error
	if s.buffer != "" {
		s.pendingRendered, cacheErr = s.render(s.buffer)
	} else {
		s.pendingRendered = ""
	}

	// Now we need to render the flushed content.
	output, err := s.render(toFlush)
	if err != nil {
		return nil, err
	}
	// If streaming cache failed, we still returning the valid flushed block, but return the error
	if cacheErr != nil {
		return nil, cacheErr
	}

	output = strings.TrimRight(output, "\n")

	return []string{output}, nil
}

func (s *streamingMarkdown) render(markdown string) (string, error) {
	if strings.TrimSpace(markdown) == "" {
		return "", nil
	}
	return s.glamour.Render(markdown)
}

// adjustBlockStart walks backwards from contentStart to find the syntactic start of the block.
// Goldmark's Lines().Start points to the text content, often skipping markers like '### ', '> ', '1. '.
func adjustBlockStart(src []byte, node ast.Node, contentStart int) int {
	// 1. Scan back through safe whitespace (spaces/tabs) on the same line
	// We want to be careful not to cross newlines yet unless the block type warrants it
	curr := contentStart
	for curr > 0 {
		c := src[curr-1]
		if c == ' ' || c == '\t' {
			curr--
		} else {
			break
		}
	}

	// 2. Handle specific block types structure
	switch node.Kind() {
	case ast.KindHeading:
		// ATX Heading: expect '#' chars
		// Scan back over hash marks
		for curr > 0 && src[curr-1] == '#' {
			curr--
		}
	case ast.KindBlockquote:
		// Blockquote: expect '>' chars
		for curr > 0 && src[curr-1] == '>' {
			curr--
		}
	case ast.KindListItem:
		// List Item: expect '-', '*', '+', or digits + '.'/'('
		// This is tricky because it could be "123. "
		// We scan back for the marker.
		scan := curr
		// Scan back over digits and dots/parens
		for scan > 0 {
			c := src[scan-1]
			if (c >= '0' && c <= '9') || c == '.' || c == ')' || c == '-' || c == '*' || c == '+' {
				scan--
			} else {
				break
			}
		}
		// If we moved, update curr
		if scan < curr {
			curr = scan
		}
	case ast.KindFencedCodeBlock:
		// Fenced Code Block: content starts after the fence line + newline
		// We need to find the specific fence line start.
		// Scan back until we find the newline that precedes the fence line
		// or start of doc.
		// The contentStart for fenced code usually starts AFTER the first newline.
		// So we just need to include the line before it.

		// Scan back to find newline
		for curr > 0 && src[curr-1] != '\n' {
			curr--
		}
		// If we found newline, that's the start of the content line.
		// But the fence is on the PREVIOUS line.
		// So we need to go back one more line.
		if curr > 0 && src[curr-1] == '\n' {
			curr--
			// Scan back to start of THAT line
			for curr > 0 && src[curr-1] != '\n' {
				curr--
			}
		}

	case ast.KindParagraph:
		// Paragraphs inside Blockquotes need to scan back over the '>' markers.
		if node.Parent() != nil && node.Parent().Kind() == ast.KindBlockquote {
			for curr > 0 {
				c := src[curr-1]
				if c == '>' || c == ' ' || c == '\t' {
					curr--
				} else {
					break
				}
			}
		}
	}

	return curr
}
