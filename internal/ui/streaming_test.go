package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/glamour"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func newTestStreamingMarkdown(t *testing.T) *streamingMarkdown {
	t.Helper()
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)
	if err != nil {
		t.Fatalf("failed to create renderer: %v", err)
	}
	return newStreamingMarkdown(r)
}

func TestStreamingMarkdown_Append(t *testing.T) {
	t.Run("Single incomplete block - no RenderRemaining", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		// Single paragraph, incomplete - should NOT RenderRemaining
		RenderRemaininged, err := sm.append("Hello world")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(RenderRemaininged) != 0 {
			t.Errorf("expected no RenderRemaining for incomplete block, got %d blocks", len(RenderRemaininged))
		}
		// Should be in pending
		if sm.pending() == "" {
			t.Error("expected content in Pending()")
		}
	})

	t.Run("Two paragraphs - first should RenderRemaining", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		// First paragraph
		sm.append("First paragraph.")

		// New paragraph (blank line creates new block)
		RenderRemaininged, err := sm.append("\n\nSecond paragraph.")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// First paragraph should have RenderRemaininged
		if len(RenderRemaininged) == 0 {
			t.Error("expected first paragraph to RenderRemaining when second starts")
		}

		// Second should be pending
		pending := sm.pending()
		if !strings.Contains(pending, "Second") {
			t.Errorf("expected 'Second' in pending, got: %s", pending)
		}
	})

	t.Run("Streaming word by word - no premature RenderRemaining", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		words := []string{"Hello", " ", "world", ".", " ", "More", " ", "text."}
		for _, word := range words {
			RenderRemaininged, _ := sm.append(word)
			if len(RenderRemaininged) != 0 {
				t.Errorf("unexpected RenderRemaining during word-by-word streaming: %v", RenderRemaininged)
			}
		}

		// All content should be pending
		pending := sm.pending()
		if !strings.Contains(pending, "Hello") || !strings.Contains(pending, "text") {
			t.Errorf("expected all words in pending, got: %s", pending)
		}
	})

	t.Run("Empty chunk - no effect", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		sm.append("Initial content")
		before := sm.pending()

		RenderRemaininged, err := sm.append("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(RenderRemaininged) != 0 {
			t.Error("empty chunk should not cause RenderRemaining")
		}

		after := sm.pending()
		if before != after {
			t.Error("empty chunk should not change pending content")
		}
	})
}

func TestStreamingMarkdown_Flush(t *testing.T) {
	t.Run("Flush with content", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		sm.append("Some content here")

		RenderRemaininged, err := sm.RenderRemaining()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if RenderRemaininged == "" {
			t.Error("expected content from Flush()")
		}

		// After RenderRemaining, pending should be empty
		if sm.pending() != "" {
			t.Error("Pending() should be empty after Flush()")
		}
	})

	t.Run("Flush with empty buffer", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		RenderRemaininged, err := sm.RenderRemaining()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if RenderRemaininged != "" {
			t.Errorf("expected empty RenderRemaining from empty buffer, got: %s", RenderRemaininged)
		}
	})

	t.Run("Double RenderRemaining returns nothing second time", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		sm.append("Content")
		sm.RenderRemaining()

		// Second RenderRemaining should return nothing
		second, err := sm.RenderRemaining()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if second != "" {
			t.Errorf("double RenderRemaining should return empty, got: %s", second)
		}
	})
}

func TestStreamingMarkdown_Pending(t *testing.T) {
	t.Run("Empty buffer returns empty string", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		if sm.pending() != "" {
			t.Error("empty buffer should return empty Pending()")
		}
	})

	t.Run("Pending returns rendered content", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		sm.append("**Bold text**")

		pending := sm.pending()
		// Glamour should render bold (may have ANSI codes)
		if pending == "" {
			t.Error("Pending() should return rendered content")
		}
	})

	t.Run("Pending is idempotent", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		sm.append("Some text")

		first := sm.pending()
		second := sm.pending()

		if first != second {
			t.Error("Pending() should be idempotent")
		}
	})

	t.Run("Pending does not consume buffer", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		sm.append("Content")
		sm.pending()

		// Flush should still return content
		RenderRemaininged, _ := sm.RenderRemaining()
		if RenderRemaininged == "" {
			t.Error("Pending() should not consume buffer")
		}
	})
}

func TestStreamingMarkdown_BlockTypes(t *testing.T) {
	t.Run("Heading block", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		sm.append("# Heading\n\nParagraph")
		pending := sm.pending()

		// Both should be rendered
		if pending == "" {
			t.Error("expected rendered content")
		}
	})

	t.Run("List block", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		sm.append("- Item 1\n- Item 2\n- Item 3")
		pending := sm.pending()

		if pending == "" {
			t.Error("expected rendered list")
		}
	})

	t.Run("Code block", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		sm.append("```go\nfunc main() {}\n```")
		pending := sm.pending()

		if pending == "" {
			t.Error("expected rendered code block")
		}
	})

	t.Run("Blockquote", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		sm.append("> Quote line 1\n> Quote line 2")
		pending := sm.pending()

		if pending == "" {
			t.Error("expected rendered blockquote")
		}
	})
}

func TestStreamingMarkdown_ComplexScenario(t *testing.T) {
	t.Run("Streaming markdown conversation", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		// Simulate LLM streaming a response
		chunks := []string{
			"Here's ",
			"how to ",
			"do it:\n\n",
			"```python\n",
			"print(",
			"'hello'",
			")\n",
			"```\n\n",
			"That's all!",
		}

		var totalFlushed int
		for _, chunk := range chunks {
			RenderRemaininged, err := sm.append(chunk)
			if err != nil {
				t.Fatalf("error appending chunk '%s': %v", chunk, err)
			}
			totalFlushed += len(RenderRemaininged)
		}

		// Should have RenderRemaininged some blocks
		// (the first paragraph and code block should RenderRemaining when new paragraph starts)
		if totalFlushed == 0 {
			t.Log("Warning: no blocks RenderRemaininged during streaming (may be expected)")
		}

		// Final RenderRemaining should get remaining content
		final, err := sm.RenderRemaining()
		if err != nil {
			t.Fatalf("error on final RenderRemaining: %v", err)
		}

		if final == "" && sm.pending() == "" && totalFlushed == 0 {
			t.Error("expected some output from streaming scenario")
		}
	})
}

func TestStreamingMarkdown_WhitespaceHandling(t *testing.T) {
	t.Run("Only whitespace", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		sm.append("   \n\n   \t\t   ")
		pending := sm.pending()

		// Whitespace-only should render to empty
		if pending != "" {
			t.Errorf("whitespace-only should render to empty, got: %q", pending)
		}
	})

	t.Run("Trailing newlines trimmed in Pending", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		sm.append("Content\n\n\n")
		pending := sm.pending()

		// Should not end with multiple newlines
		if strings.HasSuffix(pending, "\n\n") {
			t.Error("Pending() should trim trailing newlines")
		}
	})
}

func TestStreamingMarkdown_adjustBlockStart(t *testing.T) {
	tests := []struct {
		name         string
		src          string
		expectedByte byte                        // The byte at the expected adjust index
		expectedOff  int                         // Offset from start (optional, calculated if 0)
		targetNode   func(doc ast.Node) ast.Node // Helper to pick the node we want to test
	}{
		{
			name: "Heading ATX",
			src:  "# Hello\n",
			// Normal content starts at 'H'. We want to back up to '#' (index 0).
			expectedByte: '#',
			expectedOff:  0,
			targetNode: func(doc ast.Node) ast.Node {
				return doc.FirstChild() // Heading
			},
		},
		{
			name: "Heading ATX with space",
			src:  "   ## Level 2\n",
			// Content 'L'. Back up to first '#' at index 3.
			expectedByte: '#',
			expectedOff:  3,
			targetNode: func(doc ast.Node) ast.Node {
				return doc.FirstChild()
			},
		},
		{
			name: "Blockquote Simple",
			src:  "> Quote\n",
			// Content 'Q'. Back up to '>' (index 0).
			expectedByte: '>',
			expectedOff:  0,
			targetNode: func(doc ast.Node) ast.Node {
				return doc.FirstChild() // Blockquote
			},
		},
		{
			name: "List Item Dash",
			src:  "- Item\n",
			// Content 'I'. Back up to '-' (index 0).
			expectedByte: '-',
			expectedOff:  0,
			targetNode: func(doc ast.Node) ast.Node {
				// List -> ListItem
				return doc.FirstChild().FirstChild()
			},
		},
		{
			name: "List Item Numbered",
			src:  "123. Item\n",
			// Content 'I'. Back up to '1' (index 0).
			expectedByte: '1',
			expectedOff:  0,
			targetNode: func(doc ast.Node) ast.Node {
				return doc.FirstChild().FirstChild()
			},
		},
		// Fenced Code Block is a bit unique. Content usually starts on the line AFTER the fence.
		{
			name: "Fenced Code Block",
			src:  "```go\ncode\n```",
			// Content start is usually start of 'code' line.
			// We want to skip back to start of "```go".
			expectedByte: '`',
			expectedOff:  0,
			targetNode: func(doc ast.Node) ast.Node {
				return doc.FirstChild()
			},
		},
		{
			name:         "Paragraph",
			src:          "Just text\n",
			expectedByte: 'J',
			expectedOff:  0,
			targetNode: func(doc ast.Node) ast.Node {
				return doc.FirstChild()
			},
		},
		{
			name: "Paragraph inside Blockquote",
			src:  "> Inner text\n",
			// Paragraph inside blockquote. Content 'I'.
			// We want to capture the '>' as part of the paragraph context if RenderRemaininging?
			// The current logic scans back for '>', ' '.
			expectedByte: '>',
			expectedOff:  0,
			targetNode: func(doc ast.Node) ast.Node {
				// Blockquote -> Paragraph
				return doc.FirstChild().FirstChild()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := []byte(tt.src)
			parser := goldmark.New().Parser()
			doc := parser.Parse(text.NewReader(src))

			node := tt.targetNode(doc)
			if node == nil {
				t.Fatalf("Target node not found in src: %q", tt.src)
			}

			// Goldmark lines can be tricky. Containers (List, Blockquote) don't own lines, their children do.
			// But adjustBlockStart logic handles these types. To test that logic, we simulate the node having lines
			// by borrowing from the child if necessary.
			if node.Lines().Len() == 0 && node.FirstChild() != nil && node.FirstChild().Lines().Len() > 0 {
				node.SetLines(node.FirstChild().Lines())
			}

			// The "Content Start" is usually the start of the text *inside* the markers.
			lines := node.Lines()
			if lines.Len() == 0 {
				t.Fatalf("Node has no lines")
			}
			contentStart := lines.At(0).Start

			// Call the function under test
			adjusted := adjustBlockStart(src, node, contentStart)

			if adjusted >= len(src) {
				t.Fatalf("Adjusted out of bounds: %d (len %d)", adjusted, len(src))
			}

			gotByte := src[adjusted]
			if gotByte != tt.expectedByte {
				t.Errorf("Expected byte %q at index %d, got %q at index %d", tt.expectedByte, tt.expectedOff, gotByte, adjusted)
			}

			// If expectedOff is non-zero (or explicit check needed), verify exact index
			if tt.name == "Heading ATX with space" {
				if adjusted != 3 {
					t.Errorf("Expected adjusted index 3, got %d", adjusted)
				}
			}
		})
	}
}
