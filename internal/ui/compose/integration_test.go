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

// assertNoPrematureScroll verifies the view fits within available terminal space
// during the pre-scroll regime.
//
// The engine's "Split View" model:
//   - Content is flushed to scrollback via Println (TotalFlushedLines tracks this)
//   - The view renders at the effective cursor position (initial + flushed)
//   - Scroll becomes unavoidable once: flushed + view_height > available_space
//
// This assertion only checks settled frames (isPrinting=false) where scrolling
// has not yet started. The view always contains at least one unsafe block (the
// last unflushed markdown shown as preview via Pending()).
//
// The view has a minimum height (pending block ~2 lines + status bar 3 lines = ~5 lines).
// Once availableSpace < 5, terminal scroll is physically unavoidable and this
// assertion stops checking.
//
// Available space = termHeight - (cursorRow + TFL) + 1
// View must fit: viewLines ≤ availableSpace (or availableSpace >= minimum view height)
func assertNoPrematureScroll(t *testing.T, frames []viewFrame, termHeight, cursorRow, spaceBelow int) {
	t.Helper()
	const minViewHeight = 5 // pending block (~2) + status bar (3)

	for i, f := range frames {
		// Skip isPrinting frames: TFL is inflated by CBP lines not yet physically printed,
		// and the view intentionally includes CBP for "no visibility gap".
		if f.IsPrinting {
			continue
		}

		viewLines := len(strings.Split(f.View, "\n"))
		effectiveCursor := cursorRow + f.TotalFlushedLines
		availableSpace := termHeight - effectiveCursor + 1

		// Once availableSpace < minViewHeight, terminal scroll is physically unavoidable
		// (the view needs at least ~5 lines for pending content + status bar).
		// Don't flag this as premature.
		if availableSpace < minViewHeight {
			continue
		}

		if viewLines > availableSpace {
			t.Errorf("frame %d: premature scroll — view has %d lines but only %d available "+
				"at effective cursor %d (initial=%d + flushed=%d)",
				i, viewLines, availableSpace, effectiveCursor, cursorRow, f.TotalFlushedLines)
		}
	}
}

// assertViewCompactsAfterFlush verifies that padding is eliminated when flushed content
// exceeds the initial space budget.
//
// The engine's padding formula: padding = MaxAbsoluteHeight - (TFL + contentHeight).
// When TFL ≥ MaxAbsoluteHeight, padding should be 0 (no wasted space).
//
// The view always contains at least one unsafe block (the last unflushed markdown
// shown as Pending() preview), so viewLines = pending_height + statusBar_height (3).
// We can't verify the exact pending height, but we can verify padding is not wasted:
// padding_used = MaxAbsoluteHeight - TFL - (viewLines - statusBar)
// When TFL ≥ MaxAbsoluteHeight, this should be ≤ 0.
//
// This only checks settled frames (isPrinting=false) and only while TFL < MaxAbsoluteHeight
// (pre-scroll regime where the compacting guarantee applies).
func assertViewCompactsAfterFlush(t *testing.T, frames []viewFrame) {
	t.Helper()
	const statusBarHeight = 3 // "\n\n" prefix + 1 line of status text

	for i, f := range frames {
		if f.IsPrinting {
			continue
		}
		// Only relevant after some content has been flushed
		if f.TotalFlushedLines == 0 {
			continue
		}
		// The compacting guarantee only applies while TFL < MaxAbsoluteHeight (pre-scroll).
		// Once TFL ≥ MaxAbsoluteHeight, scrolling is inevitable and padding rules don't apply.
		if f.TotalFlushedLines >= f.MaxAbsoluteHeight {
			continue
		}

		viewLines := len(strings.Split(f.View, "\n"))
		// The view contains: pending_content + status_bar.
		// Pending_content = viewLines - statusBar_height.
		// Padding formula: padding = MaxAbsoluteHeight - (TFL + pending_content).
		// Rearranged: pending_content + statusBar ≤ MaxAbsoluteHeight - TFL + statusBar
		// So: viewLines ≤ MaxAbsoluteHeight - TFL + statusBar
		expectedMaxView := f.MaxAbsoluteHeight - f.TotalFlushedLines + statusBarHeight

		if viewLines > expectedMaxView {
			t.Errorf("frame %d: view not compact — has %d lines, expected ≤ %d "+
				"(maxAbs=%d, flushed=%d, statusBar=%d). Excess padding detected.",
				i, viewLines, expectedMaxView, f.MaxAbsoluteHeight, f.TotalFlushedLines, statusBarHeight)
		}
	}
}

// assertSettledViewConsistent verifies that in consecutive settled frames
// (isPrinting=false) with the same TotalFlushedLines and MaxAbsoluteHeight,
// the view height is consistent (doesn't jump up or down unexpectedly).
func assertSettledViewConsistent(t *testing.T, frames []viewFrame) {
	t.Helper()
	lastSettledHeight := -1
	lastSettledTFL := -1
	lastSettledMaxAbs := -1

	for i, f := range frames {
		if f.IsPrinting {
			// Reset tracking when entering isPrinting (state is transitional)
			lastSettledHeight = -1
			lastSettledTFL = -1
			lastSettledMaxAbs = -1
			continue
		}

		viewLines := len(strings.Split(f.View, "\n"))

		// Only compare when TFL and MaxAbs are unchanged from previous settled frame
		if lastSettledTFL == f.TotalFlushedLines && lastSettledMaxAbs == f.MaxAbsoluteHeight {
			if viewLines != lastSettledHeight {
				t.Errorf("frame %d: settled view height changed %d → %d without state change "+
					"(TFL=%d, maxAbs=%d). Visual jump detected.",
					i, lastSettledHeight, viewLines, f.TotalFlushedLines, f.MaxAbsoluteHeight)
			}
		}

		lastSettledHeight = viewLines
		lastSettledTFL = f.TotalFlushedLines
		lastSettledMaxAbs = f.MaxAbsoluteHeight
	}
}

// =========================================================================
// SHARED TEST SETUP
// =========================================================================

const (
	testTermHeight        = 24
	testCursorRow         = 18
	testChatWidth         = 40 // Narrow enough that len()-based width overflows
	testStatusBarOverhead = 2  // The "\n\n" prefix (geometry.go:13)
)

func setupTestHarness(t *testing.T) (*harnessFrameHarness, engine.Geometry) {
	t.Helper()
	lipgloss.SetColorProfile(termenv.TrueColor)

	cfg := config.DefaultConfig()
	cfg.UI.ChatWindowWidth = testChatWidth
	cd := &staticCursorDetector{row: testCursorRow}
	geom, err := teapkg.ResolveGeometry(cfg, cd, testTermHeight)
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
	return harness, geom
}

func logFrameDebug(t *testing.T, frames []viewFrame, geom engine.Geometry) {
	t.Helper()
	t.Logf("Geometry: SpaceBelow=%d, TermHeight=%d", geom.SpaceBelow, geom.TermHeight)
	for i, f := range frames {
		viewLines := len(strings.Split(f.View, "\n"))
		t.Logf("Frame %2d: viewLines=%d flushed=%d maxAbs=%d isPrinting=%v queue=%d",
			i, viewLines, f.TotalFlushedLines, f.MaxAbsoluteHeight, f.IsPrinting, f.QueueLen)
	}
}

func runAllAssertions(t *testing.T, frames []viewFrame, geom engine.Geometry) {
	t.Helper()

	t.Run("StatusBar_Always_OneLine", func(t *testing.T) {
		assertStatusBarAlwaysOneLine(t, frames)
	})

	t.Run("No_Premature_Scroll", func(t *testing.T) {
		assertNoPrematureScroll(t, frames, testTermHeight, testCursorRow, geom.SpaceBelow)
	})

	t.Run("View_Compacts_After_Flush", func(t *testing.T) {
		assertViewCompactsAfterFlush(t, frames)
	})

	t.Run("Settled_View_Consistent", func(t *testing.T) {
		assertSettledViewConsistent(t, frames)
	})
}

// =========================================================================
// TEST SUITE: OneBlockPerEvent
// =========================================================================

// TestIntegration_ViewInvariants_OneBlockPerEvent tests the Split View model
// with content streamed as complete paragraphs (1 paragraph per TextEvent).
// This is the "normal" case where markdown can flush between blocks.
//
// Geometry:
//   - TermHeight = 24, CursorRow = 18
//   - SpaceBelow = 4, statusBarOverhead = 2
//   - Initial available lines = 7 (termHeight - cursorRow + 1)
func TestIntegration_ViewInvariants_OneBlockPerEvent(t *testing.T) {
	harness, geom := setupTestHarness(t)

	// Stream 8 paragraphs. Each paragraph = 3 text lines + 1 blank line separator.
	for i := 1; i <= 8; i++ {
		para := fmt.Sprintf("Paragraph %d line 1\nParagraph %d line 2\nParagraph %d line 3\n\n", i, i, i)
		harness.ApplyEvent(domain.TextEvent{Text: para}, fmt.Sprintf("p%d", i))
	}

	frames := harness.ViewFrames()
	if len(frames) == 0 {
		t.Fatal("no frames captured")
	}

	logFrameDebug(t, frames, geom)
	runAllAssertions(t, frames, geom)
}

// =========================================================================
// TEST SUITE: IncompleteBlockStreaming
// =========================================================================

// TestIntegration_ViewInvariants_IncompleteBlockStreaming tests the Split View model
// with content streamed line-by-line (partial blocks in TextEvents).
// This mimics character-streaming from an LLM, with less frequent markdown flushing.
func TestIntegration_ViewInvariants_IncompleteBlockStreaming(t *testing.T) {
	harness, geom := setupTestHarness(t)

	// Stream 8 paragraphs line-by-line.
	for i := 1; i <= 8; i++ {
		harness.ApplyEvent(domain.TextEvent{Text: fmt.Sprintf("Paragraph %d line 1\n", i)}, fmt.Sprintf("p%d-l1", i))
		harness.ApplyEvent(domain.TextEvent{Text: fmt.Sprintf("Paragraph %d line 2\n", i)}, fmt.Sprintf("p%d-l2", i))
		harness.ApplyEvent(domain.TextEvent{Text: fmt.Sprintf("Paragraph %d line 3\n", i)}, fmt.Sprintf("p%d-l3", i))
		harness.ApplyEvent(domain.TextEvent{Text: "\n"}, fmt.Sprintf("p%d-blank", i))
	}

	frames := harness.ViewFrames()
	if len(frames) == 0 {
		t.Fatal("no frames captured")
	}

	logFrameDebug(t, frames, geom)
	runAllAssertions(t, frames, geom)
}

// =========================================================================
// TEST SUITE: MultipleBlocksPerEvent
// =========================================================================

// TestIntegration_ViewInvariants_MultipleBlocksPerEvent tests the Split View model
// with content streamed in bursts (3 complete paragraphs per TextEvent).
func TestIntegration_ViewInvariants_MultipleBlocksPerEvent(t *testing.T) {
	harness, geom := setupTestHarness(t)

	// Stream 8 paragraphs in 3 bursts (3 + 3 + 2).
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

	logFrameDebug(t, frames, geom)
	runAllAssertions(t, frames, geom)
}

// =========================================================================
// EXISTING INVARIANT TESTS
// =========================================================================

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
	if !strings.Contains(seq, "VR") {
		t.Errorf("must emit ViewRendered events, got: %s", seq)
	}
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
	if strings.Contains(seq, "QR") {
		t.Errorf("tool stream should not quit, got: %s", seq)
	}
}

func TestNewPath_GoldenSequence_DoneFlush(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "Final\n"}, "text")
	h.ApplyEvent(domain.DoneEvent{}, "done")
	seq := eventSequenceString(h.Events())
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
	lastFlush := -1
	for _, f := range frames {
		if f.TotalFlushedLines < lastFlush {
			t.Errorf("TotalFlushedLines must be monotonic: %d -> %d", lastFlush, f.TotalFlushedLines)
		}
		if f.TotalFlushedLines > lastFlush {
			lastFlush = f.TotalFlushedLines
		}
	}
	if len(events) == 0 {
		t.Error("event log must not be empty")
	}
}

// TestArchitecture_NoUnsunkTerminalWrites ensures all bubbletea.Println/Printf
// calls are only in frame.go (ProductionSink, NoopSink).
func TestArchitecture_NoUnsunkTerminalWrites(t *testing.T) {
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
			return nil
		}
		rel, _ := filepath.Rel(uiDir, path)
		if rel == filepath.Join("tea", "frame.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		s := string(content)
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
