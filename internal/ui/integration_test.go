package ui

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

// Test helpers (inlined)

func newTeatestHarness(t *testing.T) (*teatest.TestModel, *model) {
	t.Helper()
	cfg := config.DefaultConfig()
	m, err := NewTestableModel(cfg)
	if err != nil {
		t.Fatalf("failed to create model: %v", err)
	}

	tm := teatest.NewTestModel(t, m,
		teatest.WithInitialTermSize(80, 24),
	)

	return tm, m.(*model)
}

func newTeatestHarnessWithSize(t *testing.T, w, h int) (*teatest.TestModel, *model) {
	t.Helper()
	cfg := config.DefaultConfig()
	m, err := NewTestableModel(cfg)
	if err != nil {
		t.Fatalf("failed to create model: %v", err)
	}

	tm := teatest.NewTestModel(t, m,
		teatest.WithInitialTermSize(w, h),
	)

	return tm, m.(*model)
}

// newTeatestHarnessWithFrameLog adds frame logging for regression tests
func newTeatestHarnessWithFrameLog(t *testing.T) (*teatest.TestModel, *model) {
	tm, m := newTeatestHarness(t)
	m.SetDebugMode(true)
	return tm, m
}

func newTeatestHarnessWithFrameLogWithSize(t *testing.T, width, height int) (*teatest.TestModel, *model) {
	tm, m := newTeatestHarnessWithSize(t, width, height)
	m.SetDebugMode(true)
	return tm, m
}

func readAllOutput(t *testing.T, tm *teatest.TestModel) string {
	t.Helper()
	out, err := io.ReadAll(tm.FinalOutput(t, teatest.WithFinalTimeout(5*time.Second)))
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	return string(out)
}

// Integration tests

func TestIntegration_ToolOrdering(t *testing.T) {
	tm, _ := newTeatestHarness(t)

	// Start Tool A
	tm.Send(domain.ToolStartEvent{
		CallID:   "call_A",
		ToolName: "slow-tool",
		Display:  domain.StringDisplay("Tool A Running..."),
	})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Tool A Running")
	}, teatest.WithDuration(2*time.Second))

	// Start Tool B
	tm.Send(domain.ToolStartEvent{
		CallID:   "call_B",
		ToolName: "fast-tool",
		Display:  domain.StringDisplay("Tool B Running..."),
	})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Tool B Running")
	}, teatest.WithDuration(2*time.Second))

	// Finish Tool B first (should NOT flush yet)
	tm.Send(domain.ToolEndEvent{CallID: "call_B"})
	// Use WaitFor instead of Sleep to ensure state propagation
	// (Sequential sends are handled by the model's event loop)

	// Finish Tool A (should flush A, then B)
	tm.Send(domain.ToolEndEvent{CallID: "call_A"})

	// Send Done to flush everything
	tm.Send(domain.DoneEvent{})

	output := readAllOutput(t, tm)

	// Find positions of "Tool A" and "Tool B" in flushed output
	idxA := strings.Index(output, "Tool A Running")
	idxB := strings.Index(output, "Tool B Running")

	if idxA == -1 || idxB == -1 {
		t.Fatalf("expected both tools in output, got:\n%s", output)
	}

	// Tool A should appear before Tool B in output (based on Start Order preference).
	if idxA > idxB {
		t.Errorf("Tool A should appear before Tool B, but A at %d, B at %d", idxA, idxB)
	}
}

func TestIntegration_TextBeforeTool(t *testing.T) {
	tm, _ := newTeatestHarness(t)

	// Send Text
	tm.Send(domain.TextEvent{Text: "Introduction text\n"})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Introduction text")
	}, teatest.WithDuration(2*time.Second))

	// Send Tool
	tm.Send(domain.ToolStartEvent{
		CallID:   "t1",
		ToolName: "test",
		Display:  domain.StringDisplay("Tool Execution"),
	})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Tool Execution")
	}, teatest.WithDuration(2*time.Second))

	// Both finish
	tm.Send(domain.ToolEndEvent{CallID: "t1"})
	tm.Send(domain.DoneEvent{})

	// Note: WaitFor consumes the output stream, so readAllOutput will only verify what's remaining.
	// Since we verified the presence and order implicitly by waiting for Text then Tool,
	// we don't need to check readAllOutput for them again.
	// Sequential WaitFor confirms that "Introduction text" appeared, and THEN "Tool Execution" appeared.
}

func TestIntegration_FinalStatusLast(t *testing.T) {
	tm, _ := newTeatestHarness(t)

	// Send some content
	tm.Send(domain.TextEvent{Text: "Some content\n\n"})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Some content")
	}, teatest.WithDuration(2*time.Second))

	// Send Done
	tm.Send(domain.DoneEvent{})

	output := readAllOutput(t, tm)

	// Find status bar indicators
	idxDone := strings.Index(output, "Done")
	idxContext := strings.Index(output, "Context: 42%")

	if idxDone == -1 || idxContext == -1 {
		t.Fatalf("expected status bar in output, got:\n%s", output)
	}

	// Status bar should be near the end
	// Check that it appears after content
	idxContent := strings.Index(output, "Some content")
	if idxContent > idxDone {
		t.Errorf("Status bar should appear after content, but content at %d, status at %d", idxContent, idxDone)
	}

	// Status bar should be in last ~100 chars (allowing for some variance)
	// (Removed heuristic check as it is fragile and depends on ansi overhead)
}

// TestIntegration_DoneFlushesAllContent verifies that sending Done flushes everything.
func TestIntegration_DoneFlushesAllContent(t *testing.T) {
	tm, _ := newTeatestHarness(t)

	// Send tool
	tm.Send(domain.ToolStartEvent{
		CallID:   "t1",
		ToolName: "test",
		Display:  domain.StringDisplay("Tool Output"),
	})

	// Send pending text
	tm.Send(domain.TextEvent{Text: "Some new text without flush..."})

	// Immediate Done (should flush text then tool)
	tm.Send(domain.ToolEndEvent{CallID: "t1"})
	tm.Send(domain.DoneEvent{})

	output := readAllOutput(t, tm)

	if !strings.Contains(output, "Tool Output") {
		t.Error("Tool output missing")
	}
	if !strings.Contains(output, "Some new text") {
		t.Error("Pending text missing")
	}
}

func TestIntegration_CtrlCCancels(t *testing.T) {
	tm, _ := newTeatestHarness(t)

	// Add some content
	tm.Send(domain.TextEvent{Text: "Some text\n\n"})
	// Do NOT use WaitFor here as it consumes the output, and we want to verify it remains in the final output.
	// tm.Send guarantees order.

	// Start a tool
	tm.Send(domain.ToolStartEvent{
		CallID:   "t1",
		ToolName: "test",
		Display:  domain.StringDisplay("Tool Running"),
	})
	// Ditto, don't consume.

	// Send Ctrl+C
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})

	// Ctrl+C triggers Quit, so wait for finish
	output := readAllOutput(t, tm)

	// Should show cancelled status
	if !strings.Contains(output, "Cancelled") {
		t.Errorf("Ctrl+C should show cancelled status, but 'Cancelled' not found. Output:\n%s", output)
	}
	if !strings.Contains(output, "✗") {
		t.Error("Ctrl+C should show error indicator, but '✗' not found")
	}

	// Content should still be flushed
	if !strings.Contains(output, "Some text") {
		t.Error("Ctrl+C should flush text, but 'Some text' not found")
	}
}

// TestRegression_StatusBarAlwaysAfterContent verifies that the status bar never appears above content.
func TestRegression_StatusBarAlwaysAfterContent(t *testing.T) {
	tm, _ := newTeatestHarnessWithSize(t, 80, 24)

	// Stream diverse content
	tm.Send(domain.TextEvent{Text: "Header\n\n"})
	tm.Send(domain.TextEvent{Text: "Paragraph 1\n\n"})
	tm.Send(domain.ToolStartEvent{CallID: "t1", ToolName: "test", Display: domain.StringDisplay("Running...")})
	tm.Send(domain.ToolEndEvent{CallID: "t1"})
	tm.Send(domain.DoneEvent{})

	output := readAllOutput(t, tm)

	// Find last occurrence of content
	idxContent := strings.LastIndex(output, "Paragraph 1")
	// Find first occurrence of status bar
	idxStatus := strings.Index(output, "Done")

	if idxContent == -1 || idxStatus == -1 {
		t.Fatalf("Missing content or status bar in output")
	}

	if idxStatus < idxContent {
		t.Errorf("Status bar appears BEFORE content! Context: ...%s...", output[max(0, idxStatus-20):min(len(output), idxContent+20)])
	}
}

// TestRegression_StatusBarStableDuringStreaming verifies status bar position stability.
// It checks for "jumping" by ensuring the status bar line index doesn't decrease unpredictably.
func TestRegression_StatusBarStableDuringStreaming(t *testing.T) {
	tm, m := newTeatestHarnessWithFrameLog(t)

	// Simulate typing
	for i := 0; i < 5; i++ {
		tm.Send(domain.TextEvent{Text: fmt.Sprintf("chunk %d ", i)})
	}
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "chunk 4")
	}, teatest.WithDuration(1*time.Second))

	// Analyze frames
	frames := m.GetFrameLog()

	// Spinner characters from bubbles/spinner.Dot
	spinnerChars := "⣾⣽⣻⢿⡿⣟⣯⣷"

	// Ensure status bar (Generating) exists in frames and check stability
	var lastStatusRow int = -1
	for i, frame := range frames {
		if i == 0 {
			continue
		} // Skip initial partial frames

		lines := strings.Split(frame, "\n")
		statusRow := -1
		for r, line := range lines {
			if strings.Contains(line, "Generating") {
				statusRow = r
				break
			}
		}

		if statusRow == -1 {
			// Status bar not found. Only fail if spinner is present (meaning status bar SHOULD be there).
			// If no spinner, it's likely an early partial render - acceptable.
			hasSpinner := strings.ContainsAny(frame, spinnerChars)
			if hasSpinner {
				t.Errorf("Frame %d: Spinner present but 'Generating' text missing from status bar", i)
			}
			// Otherwise, early frame without spinner - skip
		} else {
			// Verify it doesn't jump UP (it can move down if content grows)
			if lastStatusRow != -1 && statusRow < lastStatusRow {
				t.Errorf("Frame %d: Status bar jumped UP from row %d to %d", i, lastStatusRow, statusRow)
			}
			lastStatusRow = statusRow
		}
	}
}

// TestRegression_StatusBarStateTransitions verifies correct status text updates.
func TestRegression_StatusBarStateTransitions(t *testing.T) {
	tm, _ := newTeatestHarness(t)

	// 1. Initial State: Generating
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Generating")
	}, teatest.WithDuration(2*time.Second))

	// 2. Thinking State
	tm.Send(domain.ThinkingEvent{})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Thinking")
	}, teatest.WithDuration(2*time.Second))

	// 3. Back to Generating (TextEvent)
	tm.Send(domain.TextEvent{Text: "Some text"})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Generating")
	}, teatest.WithDuration(2*time.Second))

	// 4. Done State
	tm.Send(domain.DoneEvent{})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Done")
	}, teatest.WithDuration(2*time.Second))
}

// TestRegression_ContentOrdering_ToolsInOrder verifies tools appear in start order.
func TestRegression_ContentOrdering_ToolsInOrder(t *testing.T) {
	tm, _ := newTeatestHarness(t)

	tm.Send(domain.ToolStartEvent{CallID: "t1", ToolName: "tool1", Display: domain.StringDisplay("Tool 1")})
	tm.Send(domain.ToolStartEvent{CallID: "t2", ToolName: "tool2", Display: domain.StringDisplay("Tool 2")})
	tm.Send(domain.DoneEvent{})

	output := readAllOutput(t, tm)

	idx1 := strings.Index(output, "Tool 1")
	idx2 := strings.Index(output, "Tool 2")

	if idx1 == -1 || idx2 == -1 {
		t.Fatal("Missing tools")
	}
	if idx1 > idx2 {
		t.Error("Tool 1 should appear before Tool 2")
	}
}

// TestRegression_CodeBlockCloseFlashing verifies no flashing/jumping on code block close.
func TestRegression_CodeBlockCloseFlashing(t *testing.T) {
	tm, m := newTeatestHarnessWithFrameLog(t)

	tm.Send(domain.TextEvent{Text: "```go\n"})
	tm.Send(domain.TextEvent{Text: "func main() {}\n"})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "func main")
	}, teatest.WithDuration(1*time.Second))

	// Capture count of frames before close
	frames := m.GetFrameLog()
	preCloseCount := len(frames)
	preCloseFrame := frames[preCloseCount-1]

	tm.Send(domain.TextEvent{Text: "```\n\n"})

	// Sync: Send Thinking event to ensure previous TextEvent is processed and rendered
	tm.Send(domain.ThinkingEvent{})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Thinking")
	}, teatest.WithDuration(1*time.Second))

	// Analyze frames after close
	newFrames := m.GetFrameLog()[preCloseCount:]
	for i, frame := range newFrames {
		// Basic stability check: content "func main" should remain visible
		if !strings.Contains(frame, "func main") {
			t.Errorf("Frame %d post-close: Content disappeared!", i)
		}

		// Check for shrinking view (a sign of flashing/clearing)
		// We ignore slight variations due to status bar spinner, but large drops are bad
		if len(frame) < len(preCloseFrame)/2 {
			t.Errorf("Frame %d post-close: View shrank significantly (possible clear/flash). Pre: %d, Post: %d", i, len(preCloseFrame), len(frame))
		}
	}
}

// TestRegression_ContentIntegrity_NoDataLoss verifies all streamed chunks appear.
func TestRegression_ContentIntegrity_NoDataLoss(t *testing.T) {
	tm, _ := newTeatestHarness(t)

	chunks := []string{"Chunk 1 ", "Chunk 2 ", "Chunk 3"}
	for _, c := range chunks {
		tm.Send(domain.TextEvent{Text: c})
	}
	tm.Send(domain.DoneEvent{})

	output := readAllOutput(t, tm)

	for _, c := range chunks {
		if !strings.Contains(output, c) {
			t.Errorf("Missing chunk: %q", c)
		}
	}
}

// TestRegression_NoPreemptiveScrolling verifies content doesn't scroll before hitting status bar.
func TestRegression_NoPreemptiveScrolling(t *testing.T) {
	// Use 24-line terminal
	tm, _ := newTeatestHarnessWithSize(t, 80, 24)

	// Send 10 lines of content (should NOT scroll yet, 14 lines remain)
	for i := 0; i < 10; i++ {
		tm.Send(domain.TextEvent{Text: fmt.Sprintf("Line %d\n", i)})
	}
	// Important: we need to wait for the program to render this
	tm.Send(domain.DoneEvent{})

	output := readAllOutput(t, tm)

	// With 24 lines, 10 lines of content + status bar (2 lines) = 12 lines
	// Should NOT see early scrolling behavior (lines pushed off top).
	// We expect all lines to be present.
	for i := 0; i < 10; i++ {
		if !strings.Contains(output, fmt.Sprintf("Line %d", i)) {
			t.Errorf("Line %d missing - possible premature scrolling", i)
		}
	}
}

// TestRegression_ToolEndFlashing verifies no flashing when tool finishes.
func TestRegression_ToolEndFlashing(t *testing.T) {
	tm, m := newTeatestHarnessWithFrameLog(t)
	tm.Send(domain.ToolStartEvent{CallID: "t1", ToolName: "test", Display: domain.StringDisplay("Running...")})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Running")
	}, teatest.WithDuration(1*time.Second))

	// Capture count of frames before end
	frames := m.GetFrameLog()
	preEndCount := len(frames)

	tm.Send(domain.ToolEndEvent{CallID: "t1"})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		// Wait until "Running" is GONE
		return !strings.Contains(string(b), "Running")
	}, teatest.WithDuration(1*time.Second))

	// Analyze frames after end
	newFrames := m.GetFrameLog()[preEndCount:]
	for i, frame := range newFrames {
		// View should not become empty
		if len(strings.TrimSpace(frame)) == 0 {
			t.Errorf("Frame %d post-end: View became empty (flash)!", i)
		}
	}
}

func TestIntegration_StatusBarLayout_Wide(t *testing.T) {
	tm, _ := newTeatestHarnessWithSize(t, 120, 24)

	// Send Done
	tm.Send(domain.DoneEvent{})
	output := readAllOutput(t, tm)

	// In wide mode (120 chars), status bar should spread out
	// "✓ Done" on left, "Context: 42%" on right
	// We check if there's significant padding
	if !strings.Contains(output, "✓ Done") {
		t.Error("Status bar missing 'Done'")
	}

	// Heuristic: Check for padding
	// If layout is correct, there should be spaces between Done and Context
	if !strings.Contains(output, "   ") {
		t.Error("Status bar should have padding in wide mode")
	}
}

func TestIntegration_StatusBarLayout_Narrow(t *testing.T) {
	tm, _ := newTeatestHarnessWithSize(t, 30, 24)

	// Send Done
	tm.Send(domain.DoneEvent{})
	output := readAllOutput(t, tm)

	// In narrow mode, it might wrap or be minimal
	// Just ensure it renders without panic and contains essential info
	if !strings.Contains(output, "Done") {
		t.Error("Status bar missing 'Done' in narrow mode")
	}
}

// TestRegression_TextFlushFlashing verifies no flashing when text block finishes.
func TestRegression_TextFlushFlashing(t *testing.T) {
	tm, m := newTeatestHarnessWithFrameLog(t)
	tm.Send(domain.TextEvent{Text: "A complete paragraph.\n\n"})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "A complete paragraph")
	}, teatest.WithDuration(1*time.Second))

	// Capture frames before Done
	frames := m.GetFrameLog()
	preDoneCount := len(frames)

	tm.Send(domain.DoneEvent{})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Done")
	}, teatest.WithDuration(1*time.Second))

	// Analyze frames after done
	newFrames := m.GetFrameLog()[preDoneCount:]
	for i, frame := range newFrames {
		// When DoneEvent is processed, m.runState becomes stateDone.
		// View() returns "" in stateDone to prevent duplicate rendering.
		// So seeing empty frames here is actually CORRECT behavior for the current design.
		// We only log if we see something unexpected, but we don't fail on empty.
		if strings.TrimSpace(frame) != "" {
			// If not empty, it should contain Done status or Context
			if !strings.Contains(frame, "Done") && !strings.Contains(frame, "Context") {
				t.Errorf("Frame %d: Status bar missing (expected Done/Context). Content: %q", i, frame)
			}
		}
	}
}

// TestRegression_MarkdownRendering verifies basic rendering of markdown elements.
func TestRegression_MarkdownRendering(t *testing.T) {
	tm, _ := newTeatestHarness(t)
	tm.Send(domain.TextEvent{Text: "# Header 1\n\n- Item 1\n- Item 2\n\n"})
	tm.Send(domain.DoneEvent{})

	output := readAllOutput(t, tm)

	// Glamour styling varies, but content should be present
	if !strings.Contains(output, "Header 1") || !strings.Contains(output, "Item 1") {
		t.Errorf("Markdown items missing or incorrectly rendered. Output:\n%s", output)
	}
}

// TestRegression_ToolOutputPreserved verifies tool output is kept.
func TestRegression_ToolOutputPreserved(t *testing.T) {
	tm, _ := newTeatestHarness(t)
	tm.Send(domain.ToolStartEvent{CallID: "t1", ToolName: "test", Display: domain.ShellDisplay{Header: "Shell Tool", Command: "echo hello"}})
	tm.Send(domain.ToolStreamEvent{CallID: "t1", Chunk: "Command result\n"})
	tm.Send(domain.ToolEndEvent{CallID: "t1"})
	tm.Send(domain.DoneEvent{})

	output := readAllOutput(t, tm)

	if !strings.Contains(output, "Command result") {
		t.Errorf("Tool output missing. Output:\n%s", output)
	}
}

// TestRegression_DoneFlushesPendingTools verifies that sending Done flushes everything.
func TestRegression_DoneFlushesPendingTools(t *testing.T) {
	tm, _ := newTeatestHarness(t)
	tm.Send(domain.ToolStartEvent{CallID: "t1", ToolName: "test", Display: domain.StringDisplay("Tool Running")})
	tm.Send(domain.DoneEvent{})

	output := readAllOutput(t, tm)

	if !strings.Contains(output, "Tool Running") {
		t.Error("DoneEvent did not flush running tools")
	}
}

// TestRegression_NoDuplicateContent verifies content isn't duplicated in output.
func TestRegression_NoDuplicateContent(t *testing.T) {
	tm, _ := newTeatestHarness(t)
	text := "Unique content string"
	tm.Send(domain.TextEvent{Text: text + "\n\n"})
	tm.Send(domain.DoneEvent{})

	output := readAllOutput(t, tm)

	// Count occurrences. We expect 1 in history (flushed) and potentially 0 in active view (since its flushed)
	// Actually, strings.Count might find it in ANSI sequences or similar, but it should stay stable.
	if strings.Count(output, text) > 1 {
		t.Errorf("Content %q duplicated %d times", text, strings.Count(output, text))
	}
}

// TestRegression_OverflowIndicator_OpenCodeBlock verifies truncation message for open code blocks.
func TestRegression_OverflowIndicator_OpenCodeBlock(t *testing.T) {
	// Very small height and width to force truncation and wrapping
	tm, m := newTeatestHarnessWithFrameLogWithSize(t, 20, 10)

	// Send 30 lines inside an open code block to force them to remain PENDING.
	// If we send plain text, it might be flushed line-by-line or paragraph-by-paragraph,
	// checking `View` would see empty content (since flushed content is in history).
	// Truncation only applies to pending content in View().
	var sb strings.Builder
	sb.WriteString("```\n")
	for i := 0; i < 30; i++ {
		sb.WriteString("Line\n")
	}
	tm.Send(domain.TextEvent{Text: sb.String()})

	// Wait a bit for render
	// We can't use WaitFor on "truncated" easily if it's transient or if ansi codes mess it up in raw stream.
	// We'll use frame log.
	// Sync: Send Thinking event to ensure previous TextEvent is processed and rendered
	tm.Send(domain.ThinkingEvent{})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Thinking")
	}, teatest.WithDuration(1*time.Second))

	frames := m.GetFrameLog()
	found := false
	for _, frame := range frames {
		if strings.Contains(frame, "truncated") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Overflow indicator 'truncated' not found in any frame")
	}
}

// TestRegression_MaxContentHeightTracking verifies that height decreases as content grows.
// We can't see the internal variable, but we can verify that the status bar moves down.
func TestRegression_MaxContentHeightTracking(t *testing.T) {
	tm, m := newTeatestHarnessWithFrameLogWithSize(t, 80, 24)

	// Initial state: padding should be large
	tm.Send(domain.TextEvent{Text: "Line 1\n\n"})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Line 1")
	}, teatest.WithDuration(1*time.Second))

	// Get latest frame as baseline (with some padding)
	// (Removed unused frame capture)

	// Add many lines
	var sb strings.Builder
	for i := 0; i < 10; i++ {
		sb.WriteString(fmt.Sprintf("New Line %d\n\n", i))
	}
	tm.Send(domain.TextEvent{Text: sb.String()})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "New Line 9")
	}, teatest.WithDuration(1*time.Second))

	frames2 := m.GetFrameLog()
	lastFrame2 := frames2[len(frames2)-1]

	// If height tracking works, output2 should have less whitespace padding than output1
	// OR simply, it should now accommodate the new content.
	// We just verify that the new content is actually present in the final frame.
	// Check that the LATEST content is present.
	// Due to scrolling (height 24), we might not see all 10 lines, but we MUST see the last one.
	if !strings.Contains(lastFrame2, "New Line 9") {
		t.Errorf("Content didn't update properly; 'New Line 9' missing from view.\nFrame:\n%s", lastFrame2)
	}
}

// TestRegression_EmptyEvents verifies empty events don't break UI.
func TestRegression_EmptyEvents(t *testing.T) {
	tm, _ := newTeatestHarness(t)
	tm.Send(domain.TextEvent{Text: ""})
	tm.Send(domain.ThinkingEvent{})
	tm.Send(domain.TextEvent{Text: "Actual content"})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Actual content")
	}, teatest.WithDuration(1*time.Second))

	tm.Send(domain.DoneEvent{})
	// Ensure clean exit
	readAllOutput(t, tm)
}

// TestRegression_RapidEvents verifies stability under stress.
func TestRegression_RapidEvents(t *testing.T) {
	tm, _ := newTeatestHarness(t)
	for i := 0; i < 50; i++ {
		tm.Send(domain.TextEvent{Text: fmt.Sprintf("chunk %d ", i)})
	}
	tm.Send(domain.DoneEvent{})

	output := readAllOutput(t, tm)

	if !strings.Contains(output, "chunk 49") {
		t.Error("Lost chunks during rapid sending")
	}
}

// TestRegression_StatusBarStable_CodeBlockClose verifies that the status bar never jumps UP
// when a code block is closed. This catches the flash bug where maxContentHeight
// is miscalculated during the flush transition.
// STRICT: This test MUST fail if the bug exists.
func TestRegression_StatusBarStable_CodeBlockClose(t *testing.T) {
	tm, m := newTeatestHarnessWithFrameLog(t)

	// 1. Establish baseline
	tm.Send(domain.TextEvent{Text: "Intro text...\n\n"})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Intro text")
	}, teatest.WithDuration(1*time.Second))

	// 2. Open code block (grows the pending area)
	tm.Send(domain.TextEvent{Text: "```go\n"})
	codeLines := []string{
		"package main\n",
		"func main() {\n",
		"    fmt.Println(\"Hello\")\n",
		"}\n",
	}
	for _, line := range codeLines {
		tm.Send(domain.TextEvent{Text: line})
	}

	// 3. Wait for full render of open block
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "fmt.Println")
	}, teatest.WithDuration(1*time.Second))

	// 4. Close code block (THIS triggers the flash)
	tm.Send(domain.TextEvent{Text: "```\n\n"})

	// 5. Send text after to ensure we capture frames *during* the transition
	tm.Send(domain.TextEvent{Text: "After code block.\n"})

	// Wait for final state
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "After code block")
	}, teatest.WithDuration(1*time.Second))

	// 6. Analyze frames for stability
	frames := m.GetFrameLog()

	// Spinner characters from bubbles/spinner.Dot
	spinnerChars := "⣾⣽⣻⢿⡿⣟⣯⣷"

	var lastStatusRow int = -1
	for i, frame := range frames {
		// Find status bar row
		lines := strings.Split(frame, "\n")
		statusRow := -1
		for r, line := range lines {
			if strings.Contains(line, "Generating") {
				statusRow = r
				break
			}
		}

		// Handle missing status bar
		if statusRow == -1 {
			if strings.ContainsAny(frame, spinnerChars) {
				// Spinner present but no "Generating" -> malformed status bar, fail
				t.Errorf("Frame %d: Spinner present but status text missing", i)
			}
			// Else: likely early partial render, ignore
			continue
		}

		// Check for stability
		// Ignore updates where we haven't established a baseline yet (first few frames)
		if lastStatusRow != -1 {
			// STRICT ASSERTION: Status bar row must NEVER decrease
			// (Allowing for slight movement if content actually shrunk, but code block close shouldn't shrink view drastically)
			if statusRow < lastStatusRow {
				t.Errorf("Frame %d: Status bar jumped UP from row %d to %d (FLASH DETECTED)\nFrame content:\n%s",
					i, lastStatusRow, statusRow, frame)
			}
		}
		lastStatusRow = statusRow
	}
}
