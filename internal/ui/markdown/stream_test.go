package markdown

import (
	"strings"
	"testing"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/muesli/termenv"
)

func newTestRenderer(t *testing.T) Renderer {
	t.Helper()
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(styles.DarkStyle),
		glamour.WithWordWrap(80),
		glamour.WithColorProfile(termenv.TrueColor),
	)
	if err != nil {
		t.Fatalf("failed to create test renderer: %v", err)
	}
	return WrapGlamour(r)
}

func normalize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimRight(s, " \t\n")
	return s
}

func renderOneShot(t *testing.T, md string) string {
	t.Helper()
	renderer := newTestRenderer(t)
	out, err := renderer.Render(md)
	if err != nil {
		t.Fatalf("renderOneShot failed: %v", err)
	}
	return normalize(out)
}

func renderStreamed(t *testing.T, md string, chunker func(string) []string) string {
	t.Helper()
	renderer := newTestRenderer(t)
	sm := NewStream(renderer)

	var flushedBlocks []string
	chunks := chunker(md)
	for _, chunk := range chunks {
		flushed, err := sm.Append(chunk)
		if err != nil {
			t.Fatalf("append failed: %v", err)
		}
		flushedBlocks = append(flushedBlocks, flushed...)
	}

	tail, err := sm.RenderRemaining()
	if err != nil {
		t.Fatalf("RenderRemaining failed: %v", err)
	}
	if tail != "" {
		flushedBlocks = append(flushedBlocks, tail)
	}

	var result strings.Builder
	for _, block := range flushedBlocks {
		result.WriteString(block)
	}
	return normalize(result.String())
}

func chunkOneShot(md string) []string {
	return []string{md}
}

func chunkByRuneSize(md string, size int) []string {
	runes := []rune(md)
	var chunks []string
	for i := 0; i < len(runes); i += size {
		end := i + size
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}

func chunkBoundarySplits(md string) []string {
	var chunks []string
	remaining := md

	if idx := strings.Index(remaining, "\n\n"); idx != -1 {
		chunks = append(chunks, remaining[:idx+2])
		remaining = remaining[idx+2:]
	}

	if idx := strings.Index(remaining, "**"); idx != -1 && idx+2 < len(remaining) {
		chunks = append(chunks, remaining[:idx+1])
		remaining = remaining[idx+1:]
	}

	if idx := strings.Index(remaining, "```"); idx != -1 {
		if idx+3 < len(remaining) {
			chunks = append(chunks, remaining[:idx+3])
			remaining = remaining[idx+3:]
		}
	}

	if remaining != "" {
		chunks = append(chunks, remaining)
	}

	if len(chunks) == 0 {
		return []string{md}
	}
	return chunks
}

var testFixtures = map[string]string{
	"F01": "Para1\n\nPara2",
	"F02": "Para1\n\n\nPara2",
	"F03": "Para1\n\n\n\n\nPara2",
	"F04": "Heading\n===\n\nNext",
	"F05": "# Heading\n\nNext",
	"F06": "- a\n- b\n\nNext",
	"F07": "1. a\n2. b\n\nNext",
	"F08": "> q1\n> q2\n\nNext",
	"F09": "```\ncode\n```\n\nNext",
	"F10": "~~~\ncode\n~~~\n\nNext",
	"F11": "Para\n\n    code\n\nNext",
	"F12": "Line1\\\nLine2\n\nNext",
	"F13": "Text **bold** more\n\nNext",
	"F14": "`Inline `code` then\n\nNext",
	"F15": "---\n\nNext",
	"F16": "\n\nLeading blank\n\nNext",
	"F17": "Unicode: 你好 👋\n\nNext",
	"F18": "Long line " + strings.Repeat("word ", 40) + "\n\nNext",
}

func TestStreamingEquivalence_OneShot(t *testing.T) {
	for name, fixture := range testFixtures {
		t.Run(name, func(t *testing.T) {
			got := renderStreamed(t, fixture, chunkOneShot)
			want := renderOneShot(t, fixture)
			if got != want {
				t.Errorf("one-shot chunking failed\nGot:\n%s\nWant:\n%s", got, want)
			}
		})
	}
}

func TestStreamingEquivalence_RuneSize1(t *testing.T) {
	for name, fixture := range testFixtures {
		t.Run(name, func(t *testing.T) {
			got := renderStreamed(t, fixture, func(md string) []string {
				return chunkByRuneSize(md, 1)
			})
			want := renderOneShot(t, fixture)
			if got != want {
				t.Errorf("rune size 1 failed\nGot:\n%s\nWant:\n%s", got, want)
			}
		})
	}
}

func TestStreamingEquivalence_RuneSize2(t *testing.T) {
	for name, fixture := range testFixtures {
		t.Run(name, func(t *testing.T) {
			got := renderStreamed(t, fixture, func(md string) []string {
				return chunkByRuneSize(md, 2)
			})
			want := renderOneShot(t, fixture)
			if got != want {
				t.Errorf("rune size 2 failed\nGot:\n%s\nWant:\n%s", got, want)
			}
		})
	}
}

func TestStreamingEquivalence_RuneSize3(t *testing.T) {
	for name, fixture := range testFixtures {
		t.Run(name, func(t *testing.T) {
			got := renderStreamed(t, fixture, func(md string) []string {
				return chunkByRuneSize(md, 3)
			})
			want := renderOneShot(t, fixture)
			if got != want {
				t.Errorf("rune size 3 failed\nGot:\n%s\nWant:\n%s", got, want)
			}
		})
	}
}

func TestStreamingEquivalence_RuneSize5(t *testing.T) {
	for name, fixture := range testFixtures {
		t.Run(name, func(t *testing.T) {
			got := renderStreamed(t, fixture, func(md string) []string {
				return chunkByRuneSize(md, 5)
			})
			want := renderOneShot(t, fixture)
			if got != want {
				t.Errorf("rune size 5 failed\nGot:\n%s\nWant:\n%s", got, want)
			}
		})
	}
}

func TestStreamingEquivalence_RuneSize8(t *testing.T) {
	for name, fixture := range testFixtures {
		t.Run(name, func(t *testing.T) {
			got := renderStreamed(t, fixture, func(md string) []string {
				return chunkByRuneSize(md, 8)
			})
			want := renderOneShot(t, fixture)
			if got != want {
				t.Errorf("rune size 8 failed\nGot:\n%s\nWant:\n%s", got, want)
			}
		})
	}
}

func TestStreamingEquivalence_RuneSize13(t *testing.T) {
	for name, fixture := range testFixtures {
		t.Run(name, func(t *testing.T) {
			got := renderStreamed(t, fixture, func(md string) []string {
				return chunkByRuneSize(md, 13)
			})
			want := renderOneShot(t, fixture)
			if got != want {
				t.Errorf("rune size 13 failed\nGot:\n%s\nWant:\n%s", got, want)
			}
		})
	}
}

func TestStreamingEquivalence_BoundarySplits(t *testing.T) {
	for name, fixture := range testFixtures {
		t.Run(name, func(t *testing.T) {
			got := renderStreamed(t, fixture, chunkBoundarySplits)
			want := renderOneShot(t, fixture)
			if got != want {
				t.Errorf("boundary splits failed\nGot:\n%s\nWant:\n%s", got, want)
			}
		})
	}
}

func TestFlush_FencedCode_IncludesClosingFence(t *testing.T) {
	renderer := newTestRenderer(t)
	sm := NewStream(renderer)

	input := "```\ncode\n```\n\nNext"
	flushed, err := sm.Append(input)
	if err != nil {
		t.Fatalf("append failed: %v", err)
	}

	if len(flushed) != 1 {
		t.Fatalf("expected 1 flushed block, got %d", len(flushed))
	}

	pending := sm.Pending()
	if strings.Contains(pending, "```") {
		t.Errorf("pending buffer should not contain closing fence, got: %q", pending)
	}
	if !strings.Contains(pending, "Next") {
		t.Errorf("pending buffer should contain 'Next', got: %q", pending)
	}
}

func TestFlush_SetextUnderline_Included(t *testing.T) {
	renderer := newTestRenderer(t)
	sm := NewStream(renderer)

	input := "Heading\n===\n\nNext"
	flushed, err := sm.Append(input)
	if err != nil {
		t.Fatalf("append failed: %v", err)
	}

	if len(flushed) != 1 {
		t.Fatalf("expected 1 flushed block, got %d", len(flushed))
	}

	pending := sm.Pending()
	if strings.Contains(pending, "===") {
		t.Errorf("pending buffer should not contain underline, got: %q", pending)
	}
	if !strings.Contains(pending, "Next") {
		t.Errorf("pending buffer should contain 'Next', got: %q", pending)
	}
}

func TestFlush_ParagraphToFence_DoesNotStealFence(t *testing.T) {
	renderer := newTestRenderer(t)
	sm := NewStream(renderer)

	input := "Para\n\n```go\ncode"
	flushed, err := sm.Append(input)
	if err != nil {
		t.Fatalf("append failed: %v", err)
	}

	if len(flushed) != 1 {
		t.Fatalf("expected 1 flushed block, got %d", len(flushed))
	}

	flushedContent := flushed[0]
	if strings.Contains(flushedContent, "```") {
		t.Errorf("flushed content should not contain opening fence, got: %q", flushedContent)
	}

	pending := sm.Pending()
	if !strings.Contains(pending, "code") {
		t.Errorf("pending buffer should contain code content, got: %q", pending)
	}
}

func TestFlush_UnclosedFence_ConsumesAll(t *testing.T) {
	renderer := newTestRenderer(t)
	sm := NewStream(renderer)

	input := "```go\ncode\n\n# NotAHeading"
	flushed, err := sm.Append(input)
	if err != nil {
		t.Fatalf("append failed: %v", err)
	}

	if len(flushed) != 0 {
		t.Errorf("unclosed fence should not flush, got %d flushed blocks", len(flushed))
	}

	pending := sm.Pending()
	if !strings.Contains(pending, "NotAHeading") {
		t.Errorf("pending buffer should contain 'NotAHeading' (consumed by code block), got: %q", pending)
	}
}

func TestFlush_ThematicBreak_NoPanic(t *testing.T) {
	renderer := newTestRenderer(t)
	sm := NewStream(renderer)

	input := "---\n\nNext"
	flushed, err := sm.Append(input)
	if err != nil {
		t.Logf("thematic break append returned error (acceptable): %v", err)
	}

	pending := sm.Pending()
	if pending == "" && len(flushed) == 0 {
		t.Log("thematic break did not flush and pending is empty (acceptable behavior)")
	}
}
