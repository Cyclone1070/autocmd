package ui

import (
	"fmt"
	"strings"
	"unicode"

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

// RenderRemaining returns any remaining content in the buffer as a single block.
func (s *streamingMarkdown) RenderRemaining() (string, error) {
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

	// Determine split index using helper
	splitIndex, err := s.findFlushSplit(src, blocks, forceFlush)
	if err != nil {
		// ...
	}
	// Debug
	if err != nil {
		// If error (e.g. can't find safe split), treat as nothing to flush
		out, renderErr := s.render(s.buffer)
		s.pendingRendered = out
		if renderErr != nil {
			return nil, renderErr
		}
		return nil, nil // Return nil error to keep waiting
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

	// Render flushed content
	output, err := s.render(toFlush)
	if err != nil {
		return nil, err
	}
	if cacheErr != nil {
		return nil, cacheErr
	}

	output = strings.TrimRight(output, "\n")
	return []string{output}, nil
}

// findFlushSplit determines the index to split the buffer for safe flushing.
// It uses "Gap Analysis" to ensure structural syntax (fences, underlines) is flushed
// with the Previous Block. It ONLY extends the split for block types that are known
// to have trailing structure excluded from Lines() (FencedCode, Setext).
func (s *streamingMarkdown) findFlushSplit(src []byte, blocks []ast.Node, forceFlush bool) (int, error) {
	if forceFlush {
		return len(src), nil
	}

	if len(blocks) <= 1 {
		return 0, fmt.Errorf("not enough blocks to flush")
	}

	safeBlock := blocks[len(blocks)-2]

	// Helper to find the LAST content end of a node
	var getLastStop func(node ast.Node) (int, error)
	getLastStop = func(node ast.Node) (int, error) {
		lines := node.Lines()
		if lines.Len() > 0 {
			lastLine := lines.At(lines.Len() - 1)
			return lastLine.Stop, nil
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
		// Thematic break might have no lines
		if node.Kind() == ast.KindThematicBreak {
			// We handle this via Kind check below, but we need a valid 'start point'
			// If it has no lines, we can't find it in src easily without parent info.
			// However, usually thematic break has lines in Goldmark if parser found it.
			// If not, we error.
			return 0, fmt.Errorf("node has no lines or children")
		}
		return 0, fmt.Errorf("node has no lines or children")
	}

	// 1. End of Safe Block Content
	safeEnd, err := getLastStop(safeBlock)
	if err != nil {
		return 0, err
	}

	// 2. Kind-Guarded Gap Analysis
	// We only scan forward if the block type likely has "Postfix Structure"
	// that was stripped by the parser.
	// - FencedCodeBlock: closing "```"
	// - Heading (Setext): underlining "==="
	// - ThematicBreak: "---" (sometimes)

	shouldScan := false
	switch safeBlock.Kind() {
	case ast.KindFencedCodeBlock:
		shouldScan = true
	case ast.KindHeading:
		// Check if Setext (heuristic: ATX starts with #)
		// We can just scan for everyone; ATX trailing "#" is harmless to include.
		// But we must NOT consume NextBlock's leading structure.
		// Since we scan to "Next Newline", ATX trailing is safe.
		// BUT Setext underline is on the NEXT line.
		shouldScan = true
	case ast.KindThematicBreak:
		shouldScan = true
	}

	if !shouldScan {
		// For Paragraphs, Lists, Quotes, etc., we assume Lines() includes everything
		// (or everything relevant).
		return safeEnd, nil
	}

	// 3. Perform Gap Analysis (Scan forward)
	// We scan from safeEnd forward. We stop at:
	// - EOF
	// - Start of Next Block (if we could detect it reliably?)
	// - A reasonable limit (struct + newline)

	// Define expected characters for structure based on Kind
	isExpectedChar := func(r rune) bool { return false }
	switch safeBlock.Kind() {
	case ast.KindFencedCodeBlock:
		isExpectedChar = func(r rune) bool { return r == '`' || r == '~' }
	case ast.KindHeading:
		isExpectedChar = func(r rune) bool { return r == '=' || r == '-' }
	case ast.KindThematicBreak:
		isExpectedChar = func(r rune) bool { return r == '-' || r == '_' || r == '*' }
	}

	splitIndex := safeEnd

	// Scan the "Gap" until we find a newline that terminates the structure
	// or we hit something that looks like the start of a next block?
	// Actually, simpler:
	// We just scan until the NEXT newline.
	// Check if that line contains non-whitespace.
	// If so, include it.

	// Loop to consume ONE line of structure (plus optional newlines?)
	// FencedCode closing fence is 1 line.
	// Setext underline is 1 line.

	current := safeEnd

	for current < len(src) {
		// Scan line
		lineStart := current
		var lineEnd int
		for i := current; i < len(src); i++ {
			if src[i] == '\n' {
				lineEnd = i + 1
				break
			}
			lineEnd = i + 1
		}

		// Check if this line is structure
		lineContent := src[lineStart:lineEnd]
		isBlank := true
		firstNonWs := rune(0)
		for _, b := range lineContent {
			r := rune(b)
			if !unicode.IsSpace(r) {
				isBlank = false
				firstNonWs = r
				break
			}
		}

		if !isBlank {
			// Found non-blank line.
			// Is it OUR structure?
			if isExpectedChar(firstNonWs) {
				// Yes. Include it.
				splitIndex = lineEnd
				break // We assume only 1 line of structure (Fence or Underline)
			} else {
				// No. It's some other content (e.g. Next Block Starts).
				// Stop scanning. Split at previous accumulated point (safeEnd or previous blank lines).
				// We do NOT include this line.
				// We also do not include preceding blank lines if we want to be strict?
				// Actually, blank lines are harmless to include in previous block usually.
				// But to be consistent with "Buffering", we should probably leave them?
				// But our splitIndex wasn't updated for blank lines.
				// So we implicitly leave blank lines in buffer. Correct.
				break
			}
		} else {
			// Blank line.
			// Could be spacing between content and fence?
			// `code\n\n``` `
			// We skip it, but do NOT update splitIndex yet.
			// If we find structure later, we will likely include this blank line?
			// No, currently we jump splitIndex to lineEnd.
			// But if we skipped intermediate blank lines, splitIndex would jump over them?
			// Logic hole: If `current` advances, but `splitIndex` stays behind...
			// If we find structure at line 3, and lines 1-2 were blank.
			// We set splitIndex = lineEnd (of line 3).
			// This includes lines 1-2. Correct.
			current = lineEnd
		}
	}

	// If we found NO structure (e.g. hit EOF), we stick to safeEnd?
	// Or we keep the 'blank lines' if we found structure after?
	// Using splitIndex update implies we include blank lines IF followed by structure.
	// If loop finishes and hasStructure is false, splitIndex remains safeEnd (correct).

	return splitIndex, nil
}

func (s *streamingMarkdown) render(markdown string) (string, error) {
	if strings.TrimSpace(markdown) == "" {
		return "", nil
	}
	return s.glamour.Render(markdown)
}
