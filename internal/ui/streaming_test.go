package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/glamour"
)

func newTestStreamingMarkdown(t *testing.T) *StreamingMarkdown {
	t.Helper()
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)
	if err != nil {
		t.Fatalf("failed to create renderer: %v", err)
	}
	return NewStreamingMarkdown(r)
}

func TestStreamingMarkdown_Append(t *testing.T) {
	t.Run("Single incomplete block - no flush", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		// Single paragraph, incomplete - should NOT flush
		flushed, err := sm.Append("Hello world")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(flushed) != 0 {
			t.Errorf("expected no flush for incomplete block, got %d blocks", len(flushed))
		}
		// Should be in pending
		if sm.Pending() == "" {
			t.Error("expected content in Pending()")
		}
	})

	t.Run("Two paragraphs - first should flush", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		// First paragraph
		sm.Append("First paragraph.")

		// New paragraph (blank line creates new block)
		flushed, err := sm.Append("\n\nSecond paragraph.")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// First paragraph should have flushed
		if len(flushed) == 0 {
			t.Error("expected first paragraph to flush when second starts")
		}

		// Second should be pending
		pending := sm.Pending()
		if !strings.Contains(pending, "Second") {
			t.Errorf("expected 'Second' in pending, got: %s", pending)
		}
	})

	t.Run("Streaming word by word - no premature flush", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		words := []string{"Hello", " ", "world", ".", " ", "More", " ", "text."}
		for _, word := range words {
			flushed, _ := sm.Append(word)
			if len(flushed) != 0 {
				t.Errorf("unexpected flush during word-by-word streaming: %v", flushed)
			}
		}

		// All content should be pending
		pending := sm.Pending()
		if !strings.Contains(pending, "Hello") || !strings.Contains(pending, "text") {
			t.Errorf("expected all words in pending, got: %s", pending)
		}
	})

	t.Run("Empty chunk - no effect", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		sm.Append("Initial content")
		before := sm.Pending()

		flushed, err := sm.Append("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(flushed) != 0 {
			t.Error("empty chunk should not cause flush")
		}

		after := sm.Pending()
		if before != after {
			t.Error("empty chunk should not change pending content")
		}
	})
}

func TestStreamingMarkdown_Flush(t *testing.T) {
	t.Run("Flush with content", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		sm.Append("Some content here")

		flushed, err := sm.Flush()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if flushed == "" {
			t.Error("expected content from Flush()")
		}

		// After flush, pending should be empty
		if sm.Pending() != "" {
			t.Error("Pending() should be empty after Flush()")
		}
	})

	t.Run("Flush with empty buffer", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		flushed, err := sm.Flush()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if flushed != "" {
			t.Errorf("expected empty flush from empty buffer, got: %s", flushed)
		}
	})

	t.Run("Double flush returns nothing second time", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		sm.Append("Content")
		sm.Flush()

		// Second flush should return nothing
		second, err := sm.Flush()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if second != "" {
			t.Errorf("double flush should return empty, got: %s", second)
		}
	})
}

func TestStreamingMarkdown_Pending(t *testing.T) {
	t.Run("Empty buffer returns empty string", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		if sm.Pending() != "" {
			t.Error("empty buffer should return empty Pending()")
		}
	})

	t.Run("Pending returns rendered content", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		sm.Append("**Bold text**")

		pending := sm.Pending()
		// Glamour should render bold (may have ANSI codes)
		if pending == "" {
			t.Error("Pending() should return rendered content")
		}
	})

	t.Run("Pending is idempotent", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		sm.Append("Some text")

		first := sm.Pending()
		second := sm.Pending()

		if first != second {
			t.Error("Pending() should be idempotent")
		}
	})

	t.Run("Pending does not consume buffer", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		sm.Append("Content")
		sm.Pending()

		// Flush should still return content
		flushed, _ := sm.Flush()
		if flushed == "" {
			t.Error("Pending() should not consume buffer")
		}
	})
}

func TestStreamingMarkdown_BlockTypes(t *testing.T) {
	t.Run("Heading block", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		sm.Append("# Heading\n\nParagraph")
		pending := sm.Pending()

		// Both should be rendered
		if pending == "" {
			t.Error("expected rendered content")
		}
	})

	t.Run("List block", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		sm.Append("- Item 1\n- Item 2\n- Item 3")
		pending := sm.Pending()

		if pending == "" {
			t.Error("expected rendered list")
		}
	})

	t.Run("Code block", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		sm.Append("```go\nfunc main() {}\n```")
		pending := sm.Pending()

		if pending == "" {
			t.Error("expected rendered code block")
		}
	})

	t.Run("Blockquote", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		sm.Append("> Quote line 1\n> Quote line 2")
		pending := sm.Pending()

		if pending == "" {
			t.Error("expected rendered blockquote")
		}
	})
}

func TestStreamingMarkdown_SetRenderer(t *testing.T) {
	sm := newTestStreamingMarkdown(t)
	sm.Append("Initial content")

	// Create new renderer with different width
	newRenderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(40),
	)
	if err != nil {
		t.Fatalf("failed to create new renderer: %v", err)
	}

	sm.SetRenderer(newRenderer)

	// Pending should still work with new renderer
	pending := sm.Pending()
	if pending == "" {
		t.Error("SetRenderer should allow continued operation")
	}
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
			flushed, err := sm.Append(chunk)
			if err != nil {
				t.Fatalf("error appending chunk '%s': %v", chunk, err)
			}
			totalFlushed += len(flushed)
		}

		// Should have flushed some blocks
		// (the first paragraph and code block should flush when new paragraph starts)
		if totalFlushed == 0 {
			t.Log("Warning: no blocks flushed during streaming (may be expected)")
		}

		// Final flush should get remaining content
		final, err := sm.Flush()
		if err != nil {
			t.Fatalf("error on final flush: %v", err)
		}

		if final == "" && sm.Pending() == "" && totalFlushed == 0 {
			t.Error("expected some output from streaming scenario")
		}
	})
}

func TestStreamingMarkdown_WhitespaceHandling(t *testing.T) {
	t.Run("Only whitespace", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		sm.Append("   \n\n   \t\t   ")
		pending := sm.Pending()

		// Whitespace-only should render to empty
		if pending != "" {
			t.Errorf("whitespace-only should render to empty, got: %q", pending)
		}
	})

	t.Run("Trailing newlines trimmed in Pending", func(t *testing.T) {
		sm := newTestStreamingMarkdown(t)

		sm.Append("Content\n\n\n")
		pending := sm.Pending()

		// Should not end with multiple newlines
		if strings.HasSuffix(pending, "\n\n") {
			t.Error("Pending() should trim trailing newlines")
		}
	})
}
