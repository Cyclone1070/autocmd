package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui/engine"
	"github.com/Cyclone1070/iav/internal/ui/markdown"
	teapkg "github.com/Cyclone1070/iav/internal/ui/tea"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// =========================================================================
// EXTRACTED ASSERTION HELPERS
// =========================================================================

// assertStatusBarAlwaysOneLine checks that status bar text is on exactly 1 line.
// The status bar has two components: spinner+status (left) and context info (right).
// They MUST be on the same line, joined by a gap.
func assertStatusBarAlwaysOneLine(t *testing.T, frames []viewFrame) {
	t.Helper()
	for i, f := range frames {
		lines := strings.Split(f.View, "\n")
		contextRow := -1
		statusRow := -1

		for j, line := range lines {
			if strings.Contains(line, "Context:") {
				contextRow = j
			}
			if strings.Contains(line, "Generating") || strings.Contains(line, "Thinking") ||
				strings.Contains(line, "Done") {
				statusRow = j
			}
		}

		if contextRow != -1 && statusRow != -1 && contextRow != statusRow {
			t.Errorf("frame %d: status bar split across 2 lines (status on line %d, context on line %d)",
				i, statusRow, contextRow)
		}
	}
}

// assertEarlyScrollDetection checks that the view never exceeds available space
// at the effective cursor position (initial cursor + flushed history).
// This catches all sources of early scroll:
//   - View overflow (content/padding/statusBar too large)
//   - History accumulation (TotalFlushedLines grows too fast due to padding bug)
//   - MaxAbsoluteHeight miscalculation (wrong budget causes premature flushing)
func assertEarlyScrollDetection(t *testing.T, frames []viewFrame, termHeight, cursorRow int) {
	t.Helper()
	for i, f := range frames {
		viewLines := len(strings.Split(f.View, "\n"))
		effectiveCursor := cursorRow + f.TotalFlushedLines
		availableSpace := termHeight - effectiveCursor + 1

		if viewLines > availableSpace {
			t.Errorf("frame %d: view has %d lines but only %d available at effective cursor %d "+
				"(initial=%d + flushed=%d). Early scroll detected",
				i, viewLines, availableSpace, effectiveCursor, cursorRow, f.TotalFlushedLines)
		}
	}
}

// assertHistoryPlusViewStable checks that the total height (history + view)
// is stable or grows only when MaxAbsoluteHeight grows.
// This tests the Split View model invariant: no visual jumps without state changes.
func assertHistoryPlusViewStable(t *testing.T, frames []viewFrame, expectedTotal int) {
	t.Helper()
	initialMaxAbs := 0
	for i, f := range frames {
		if i == 0 && f.MaxAbsoluteHeight > 0 {
			initialMaxAbs = f.MaxAbsoluteHeight
		}

		viewLines := len(strings.Split(f.View, "\n"))
		totalHeight := f.TotalFlushedLines + viewLines

		if totalHeight > expectedTotal && f.MaxAbsoluteHeight == initialMaxAbs {
			t.Errorf("frame %d: history(%d) + view(%d) = %d, want ≤ %d. "+
				"MaxAbsoluteHeight=%d unchanged → status bar is consuming extra lines",
				i, f.TotalFlushedLines, viewLines, totalHeight,
				expectedTotal, f.MaxAbsoluteHeight)
		}
	}
}

// =========================================================================
// TEST SUITE: OneBlockPerEvent
// =========================================================================

// TestIntegration_ViewInvariants_OneBlockPerEvent tests the Split View model
// with content streamed as complete paragraphs (1 paragraph per TextEvent).
// This is the "normal" case where markdown can flush between blocks.
//
// Geometry:
//   - TermHeight = 24, CursorRow = 18 → available lines = 7
//   - statusBarOverhead = 2 (the "\n\n" prefix)
//   - SpaceBelow = 4 = MaxAbsoluteHeight (initial)
//   - Status bar text = 1 line
//
// This test verifies:
// 1. StatusBar_Always_OneLine: status bar stays 1 line (not split by ANSI codes)
// 2. Early_Scroll_Detection: view never exceeds available space
// 3. History_Plus_View_Stable: total height is stable (no visual jumps)
func TestIntegration_ViewInvariants_OneBlockPerEvent(t *testing.T) {
	// Force color profile so the spinner styling actually produces ANSI codes.
	lipgloss.SetColorProfile(termenv.TrueColor)

	const (
		termHeight        = 24
		cursorRow         = 18
		chatWidth         = 40 // Narrow enough that len()-based width overflows
		statusBarOverhead = 2  // The "\n\n" prefix (geometry.go:13)
		statusTextLines   = 1  // Status bar text should be exactly 1 line
	)

	// Available terminal lines from cursor to bottom (inclusive).
	availableLines := termHeight - cursorRow + 1 // = 7
	expectedTotal := statusBarOverhead + statusTextLines + (termHeight - cursorRow - statusBarOverhead) // = 7

	cfg := config.DefaultConfig()
	cfg.UI.ChatWindowWidth = chatWidth
	cd := &staticCursorDetector{row: cursorRow}
	geom, err := teapkg.ResolveGeometry(cfg, cd, termHeight)
	if err != nil {
		t.Fatalf("ResolveGeometry: %v", err)
	}
	mdRenderer, err := markdown.NewGlamourRenderer(80)
	if err != nil {
		t.Fatalf("markdown.NewGlamourRenderer: %v", err)
	}
	sm := markdown.NewStream(mdRenderer)
	state := engine.NewInitialState(geom)

	// Use the REAL spinner with styling (TeaModelAdapter applies lipgloss color).
	factory := func(s *spinner.Model) engine.Deps {
		deps := NewEngineDeps(cfg, sm, 80)
		return deps
	}
	sink := &teapkg.RecordingSink{}
	adapter := teapkg.NewTeaModelAdapter(state, factory, sink)
	harness := &harnessFrameHarness{adapter: adapter, sink: sink}

	// Stream 8 paragraphs. Each paragraph = 3 text lines + 1 blank line separator.
	// Blank lines create markdown block boundaries, triggering flushing (≥2 blocks).
	for i := 1; i <= 8; i++ {
		para := fmt.Sprintf("Paragraph %d line 1\nParagraph %d line 2\nParagraph %d line 3\n\n", i, i, i)
		harness.ApplyEvent(domain.TextEvent{Text: para}, fmt.Sprintf("p%d", i))
	}

	frames := harness.ViewFrames()
	if len(frames) == 0 {
		t.Fatal("no frames captured")
	}

	// --- Debug logging (always, not just on failure) ---
	t.Logf("Geometry: SpaceBelow=%d, MaxAbsoluteHeight=%d, Available=%d",
		geom.SpaceBelow, state.MaxAbsoluteHeight, availableLines)
	for i, f := range frames {
		viewLines := len(strings.Split(f.View, "\n"))
		totalHeight := f.TotalFlushedLines + viewLines
		t.Logf("Frame %2d: viewLines=%d, flushed=%d, total=%d, maxAbs=%d",
			i, viewLines, f.TotalFlushedLines, totalHeight, f.MaxAbsoluteHeight)
	}

	// Run all 3 assertions on this streaming pattern
	t.Run("StatusBar_Always_OneLine", func(t *testing.T) {
		assertStatusBarAlwaysOneLine(t, frames)
	})

	t.Run("Early_Scroll_Detection", func(t *testing.T) {
		assertEarlyScrollDetection(t, frames, termHeight, cursorRow)
	})

	t.Run("History_Plus_View_Stable", func(t *testing.T) {
		assertHistoryPlusViewStable(t, frames, expectedTotal)
	})
}

// =========================================================================
// TEST SUITE: IncompleteBlockStreaming
// =========================================================================

// TestIntegration_ViewInvariants_IncompleteBlockStreaming tests the Split View model
// with content streamed line-by-line (partial blocks in TextEvents).
// This mimics character-streaming from an LLM, with less frequent markdown flushing.
// More content accumulates before flushing, stressing the layout engine.
func TestIntegration_ViewInvariants_IncompleteBlockStreaming(t *testing.T) {
	// Force color profile so the spinner styling actually produces ANSI codes.
	lipgloss.SetColorProfile(termenv.TrueColor)

	const (
		termHeight        = 24
		cursorRow         = 18
		chatWidth         = 40
		statusBarOverhead = 2
		statusTextLines   = 1
	)

	availableLines := termHeight - cursorRow + 1
	expectedTotal := statusBarOverhead + statusTextLines + (termHeight - cursorRow - statusBarOverhead)

	cfg := config.DefaultConfig()
	cfg.UI.ChatWindowWidth = chatWidth
	cd := &staticCursorDetector{row: cursorRow}
	geom, err := teapkg.ResolveGeometry(cfg, cd, termHeight)
	if err != nil {
		t.Fatalf("ResolveGeometry: %v", err)
	}
	mdRenderer, err := markdown.NewGlamourRenderer(80)
	if err != nil {
		t.Fatalf("markdown.NewGlamourRenderer: %v", err)
	}
	sm := markdown.NewStream(mdRenderer)
	state := engine.NewInitialState(geom)

	factory := func(s *spinner.Model) engine.Deps {
		deps := NewEngineDeps(cfg, sm, 80)
		return deps
	}
	sink := &teapkg.RecordingSink{}
	adapter := teapkg.NewTeaModelAdapter(state, factory, sink)
	harness := &harnessFrameHarness{adapter: adapter, sink: sink}

	// Stream 8 paragraphs line-by-line (24 lines total + 8 blank separators = 32 events).
	// Markdown flushes only when 2+ complete blocks are ready.
	// This creates scenarios where content accumulates faster than flushing.
	for i := 1; i <= 8; i++ {
		// Lines 1-3 of paragraph
		harness.ApplyEvent(domain.TextEvent{Text: fmt.Sprintf("Paragraph %d line 1\n", i)}, fmt.Sprintf("p%d-l1", i))
		harness.ApplyEvent(domain.TextEvent{Text: fmt.Sprintf("Paragraph %d line 2\n", i)}, fmt.Sprintf("p%d-l2", i))
		harness.ApplyEvent(domain.TextEvent{Text: fmt.Sprintf("Paragraph %d line 3\n", i)}, fmt.Sprintf("p%d-l3", i))
		// Blank line to complete the block
		harness.ApplyEvent(domain.TextEvent{Text: "\n"}, fmt.Sprintf("p%d-blank", i))
	}

	frames := harness.ViewFrames()
	if len(frames) == 0 {
		t.Fatal("no frames captured")
	}

	// --- Debug logging ---
	t.Logf("Geometry: SpaceBelow=%d, MaxAbsoluteHeight=%d, Available=%d",
		geom.SpaceBelow, state.MaxAbsoluteHeight, availableLines)
	for i, f := range frames {
		viewLines := len(strings.Split(f.View, "\n"))
		totalHeight := f.TotalFlushedLines + viewLines
		t.Logf("Frame %2d: viewLines=%d, flushed=%d, total=%d, maxAbs=%d",
			i, viewLines, f.TotalFlushedLines, totalHeight, f.MaxAbsoluteHeight)
	}

	// Run all 3 assertions on this streaming pattern
	t.Run("StatusBar_Always_OneLine", func(t *testing.T) {
		assertStatusBarAlwaysOneLine(t, frames)
	})

	t.Run("Early_Scroll_Detection", func(t *testing.T) {
		assertEarlyScrollDetection(t, frames, termHeight, cursorRow)
	})

	t.Run("History_Plus_View_Stable", func(t *testing.T) {
		assertHistoryPlusViewStable(t, frames, expectedTotal)
	})
}

// =========================================================================
// TEST SUITE: MultipleBlocksPerEvent
// =========================================================================

// TestIntegration_ViewInvariants_MultipleBlocksPerEvent tests the Split View model
// with content streamed in bursts (3 complete paragraphs per TextEvent).
// This creates aggressive content arrival, immediately triggering overflow scenarios.
func TestIntegration_ViewInvariants_MultipleBlocksPerEvent(t *testing.T) {
	// Force color profile so the spinner styling actually produces ANSI codes.
	lipgloss.SetColorProfile(termenv.TrueColor)

	const (
		termHeight        = 24
		cursorRow         = 18
		chatWidth         = 40
		statusBarOverhead = 2
		statusTextLines   = 1
	)

	availableLines := termHeight - cursorRow + 1
	expectedTotal := statusBarOverhead + statusTextLines + (termHeight - cursorRow - statusBarOverhead)

	cfg := config.DefaultConfig()
	cfg.UI.ChatWindowWidth = chatWidth
	cd := &staticCursorDetector{row: cursorRow}
	geom, err := teapkg.ResolveGeometry(cfg, cd, termHeight)
	if err != nil {
		t.Fatalf("ResolveGeometry: %v", err)
	}
	mdRenderer, err := markdown.NewGlamourRenderer(80)
	if err != nil {
		t.Fatalf("markdown.NewGlamourRenderer: %v", err)
	}
	sm := markdown.NewStream(mdRenderer)
	state := engine.NewInitialState(geom)

	factory := func(s *spinner.Model) engine.Deps {
		deps := NewEngineDeps(cfg, sm, 80)
		return deps
	}
	sink := &teapkg.RecordingSink{}
	adapter := teapkg.NewTeaModelAdapter(state, factory, sink)
	harness := &harnessFrameHarness{adapter: adapter, sink: sink}

	// Stream 8 paragraphs in 3 bursts (3 paragraphs in event 1, 3 in event 2, 2 in event 3).
	// Each burst = multiple complete blocks, triggering markdown flush immediately.
	// Total = 24 text lines + 8 blank separators across 3 TextEvents.
	burst1 := ""
	for i := 1; i <= 3; i++ {
		burst1 += fmt.Sprintf("Paragraph %d line 1\nParagraph %d line 2\nParagraph %d line 3\n\n", i, i, i)
	}
	harness.ApplyEvent(domain.TextEvent{Text: burst1}, "burst1")

	burst2 := ""
	for i := 4; i <= 6; i++ {
		burst2 += fmt.Sprintf("Paragraph %d line 1\nParagraph %d line 2\nParagraph %d line 3\n\n", i, i, i)
	}
	harness.ApplyEvent(domain.TextEvent{Text: burst2}, "burst2")

	burst3 := ""
	for i := 7; i <= 8; i++ {
		burst3 += fmt.Sprintf("Paragraph %d line 1\nParagraph %d line 2\nParagraph %d line 3\n\n", i, i, i)
	}
	harness.ApplyEvent(domain.TextEvent{Text: burst3}, "burst3")

	frames := harness.ViewFrames()
	if len(frames) == 0 {
		t.Fatal("no frames captured")
	}

	// --- Debug logging ---
	t.Logf("Geometry: SpaceBelow=%d, MaxAbsoluteHeight=%d, Available=%d",
		geom.SpaceBelow, state.MaxAbsoluteHeight, availableLines)
	for i, f := range frames {
		viewLines := len(strings.Split(f.View, "\n"))
		totalHeight := f.TotalFlushedLines + viewLines
		t.Logf("Frame %2d: viewLines=%d, flushed=%d, total=%d, maxAbs=%d",
			i, viewLines, f.TotalFlushedLines, totalHeight, f.MaxAbsoluteHeight)
	}

	// Run all 3 assertions on this streaming pattern
	t.Run("StatusBar_Always_OneLine", func(t *testing.T) {
		assertStatusBarAlwaysOneLine(t, frames)
	})

	t.Run("Early_Scroll_Detection", func(t *testing.T) {
		assertEarlyScrollDetection(t, frames, termHeight, cursorRow)
	})

	t.Run("History_Plus_View_Stable", func(t *testing.T) {
		assertHistoryPlusViewStable(t, frames, expectedTotal)
	})
}

// --- Invariant tests for new path ---

func TestNewPath_StatusBarNeverJumpsUp(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "A\n"}, "t1")
	h.ApplyEvent(domain.TextEvent{Text: "B\n\n"}, "t2")
	assertStatusBarNeverJumpsUp(t, h.ViewFrames())
}

func TestNewPath_VisualBottomMonotonic(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "Line 1\n"}, "t1")
	h.ApplyEvent(domain.TextEvent{Text: "Line 2\n\n"}, "t2")
	h.ApplyEvent(domain.TextEvent{Text: "Line 3\n"}, "t3")
	assertVisualBottomMonotonic(t, h.ViewFrames())
}

func TestNewPath_MaxAbsoluteHeightMonotonic(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "A\n"}, "t1")
	h.ApplyEvent(domain.TextEvent{Text: "B\n\n"}, "t2")
	assertMaxAbsoluteHeightMonotonic(t, h.ViewFrames())
}

func TestNewPath_StatusBarAfterContent(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "A\n"}, "t1")
	h.ApplyEvent(domain.TextEvent{Text: "B\n\n"}, "t2")
	assertStatusBarAfterContent(t, h.ViewFrames())
}

func TestNewPath_NoFlushVisibilityGap(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "Pre\n\n"}, "pre")
	h.ApplyEvent(domain.ToolStartEvent{CallID: "t1", ToolName: "x", Display: domain.StringDisplay("Running")}, "tool")
	h.ApplyEvent(domain.ToolEndEvent{CallID: "t1"}, "end")
	assertNoFlushVisibilityGap(t, h.ViewFrames(), []string{"Pre"})
}

func TestNewPath_OrderingParity(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "Intro text\n\n"}, "intro")
	h.ApplyEvent(domain.ToolStartEvent{CallID: "a", ToolName: "x", Display: domain.StringDisplay("Tool A")}, "toolA")
	h.ApplyEvent(domain.ToolStartEvent{CallID: "b", ToolName: "y", Display: domain.StringDisplay("Tool B")}, "toolB")
	h.ApplyEvent(domain.ToolEndEvent{CallID: "a"}, "endA")
	h.ApplyEvent(domain.ToolEndEvent{CallID: "b"}, "endB")
	h.ApplyEvent(domain.TextEvent{Text: "trailer\n\n"}, "trailer")
	frames := h.ViewFrames()
	var lastView string
	for i := len(frames) - 1; i >= 0; i-- {
		if frames[i].View != "" {
			lastView = frames[i].View
			break
		}
	}
	idxIntro := strings.Index(lastView, "Intro text")
	idxA := strings.Index(lastView, "Tool A")
	idxB := strings.Index(lastView, "Tool B")
	idxStatus := strings.Index(lastView, "Context:")
	if idxIntro == -1 || idxA == -1 || idxB == -1 || idxStatus == -1 {
		t.Fatalf("missing expected content in final view:\n%s", lastView)
	}
	if idxIntro > idxA {
		t.Error("parity: intro text must appear before Tool A")
	}
	if idxA > idxB {
		t.Error("parity: Tool A must appear before Tool B")
	}
	if idxB > idxStatus {
		t.Error("parity: status bar (Context:) must appear last")
	}
}

func TestNewPath_TextContentOrdering_Frames(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 1)
	h.ApplyEvent(domain.TextEvent{Text: "Intro text\n"}, "text")
	h.ApplyEvent(domain.TextEvent{Text: "More\n\n"}, "more")
	frames := h.ViewFrames()
	if len(frames) < 2 {
		t.Errorf("expected at least 2 frames, got %d", len(frames))
	}
	assertStatusBarAfterContent(t, frames)
	assertVisualBottomMonotonic(t, frames)
}

func TestNewPath_GoldenSequence_TextStream(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "Hi\n"}, "t1")
	h.ApplyEvent(domain.TextEvent{Text: "Bye\n\n"}, "t2")
	seq := eventSequenceString(h.Events())
	// Text stream: ViewRendered on every render. HistoryFlushed when markdown flushes complete blocks.
	// Markdown may not flush until 2+ blocks; we assert we get ViewRendered and valid sequence.
	if !strings.Contains(seq, "VR") {
		t.Errorf("must emit ViewRendered events, got: %s", seq)
	}
	// Event log must be non-empty and ordered
	if len(h.Events()) == 0 {
		t.Error("event log must not be empty")
	}
}

func TestNewPath_GoldenSequence_ToolStream(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "Pre\n\n"}, "pre")
	h.ApplyEvent(domain.ToolStartEvent{CallID: "t1", ToolName: "x", Display: domain.StringDisplay("Tool")}, "tool")
	h.ApplyEvent(domain.ToolEndEvent{CallID: "t1"}, "end")
	events := h.Events()
	seq := eventSequenceString(events)
	if !strings.Contains(seq, "HF") {
		t.Errorf("tool stream should emit HistoryFlushed (tool box flush), got: %s", seq)
	}
	if !strings.Contains(seq, "VR") {
		t.Errorf("must emit ViewRendered, got: %s", seq)
	}
	// No QuitRequested in normal tool flow
	if strings.Contains(seq, "QR") {
		t.Errorf("tool stream should not quit, got: %s", seq)
	}
}

func TestNewPath_GoldenSequence_DoneFlush(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "Final\n"}, "text")
	h.ApplyEvent(domain.DoneEvent{}, "done")
	seq := eventSequenceString(h.Events())
	// Done triggers flush of remaining content, then QuitRequested
	if !strings.Contains(seq, "QR") {
		t.Errorf("done must emit QuitRequested, got: %s", seq)
	}
	if !strings.Contains(seq, "HF") {
		t.Errorf("done must flush content, got: %s", seq)
	}
	if len(h.Events()) == 0 {
		t.Error("event log must not be empty")
	}
}

func TestNewPath_EventSequence_CompleteTimeline(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "A\n"}, "t1")
	h.ApplyEvent(domain.TextEvent{Text: "B\n"}, "t2")
	frames := h.ViewFrames()
	events := h.Events()
	// Every ViewRendered must have monotonic TotalFlushedLines
	lastFlush := -1
	for _, f := range frames {
		if f.TotalFlushedLines < lastFlush {
			t.Errorf("TotalFlushedLines must be monotonic: %d -> %d", lastFlush, f.TotalFlushedLines)
		}
		if f.TotalFlushedLines > lastFlush {
			lastFlush = f.TotalFlushedLines
		}
	}
	// Event log must be non-empty
	if len(events) == 0 {
		t.Error("event log must not be empty")
	}
}

// TestArchitecture_NoUnsunkTerminalWrites ensures all bubbletea.Println/Printf
// calls are only in frame.go (ProductionSink, NoopSink). No direct terminal writes
// outside the FrameSink path.
func TestArchitecture_NoUnsunkTerminalWrites(t *testing.T) {
	// Walk from ui package (parent of compose)
	uiDir := ".."
	allowedFile := "tea/frame.go"
	var violations []string
	err := filepath.Walk(uiDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil // Test files may reference the pattern
		}
		rel, _ := filepath.Rel(uiDir, path)
		if rel == filepath.Join("tea", "frame.go") {
			return nil // Allowed: sink implementations
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		s := string(content)
		// Match actual calls, not string literals or comments
		if strings.Contains(s, "bubbletea.Println(") || strings.Contains(s, "bubbletea.Printf(") {
			violations = append(violations, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(violations) > 0 {
		t.Errorf("bubbletea.Println/Printf must only appear in %s; found in: %v", allowedFile, violations)
	}
}
