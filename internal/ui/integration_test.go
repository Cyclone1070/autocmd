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

// staticCursorDetector allows tests to control the reported cursor row.
type staticCursorDetector struct {
	row int
}

func (d *staticCursorDetector) GetCursorRow() (int, error) {
	return d.row, nil
}

// NewTestableModelWithCursorRow allows tests to specify exact cursor position.
// Use row=height to simulate "at bottom" (no padding).
func NewTestableModelWithCursorRow(cfg *config.Config, row int) (tea.Model, error) {
	cd := &staticCursorDetector{row: row}
	return newModel(cfg, cd)
}

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

func newTeatestHarnessWithFrameLogAndCursor(t *testing.T, width, height, cursorRow int) (*teatest.TestModel, *model) {
	t.Helper()
	cfg := config.DefaultConfig()
	mRaw, err := NewTestableModelWithCursorRow(cfg, cursorRow)
	if err != nil {
		t.Fatalf("failed to create model: %v", err)
	}

	m := mRaw.(*model)
	m.SetDebugMode(true)

	tm := teatest.NewTestModel(t, m,
		teatest.WithInitialTermSize(width, height),
	)

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
	// Send some content (fragmented)
	tm.Send(domain.TextEvent{Text: "Some "})
	tm.Send(domain.TextEvent{Text: "con"})
	tm.Send(domain.TextEvent{Text: "tent\n\n"})
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
	// Send pending text (fragmented)
	tm.Send(domain.TextEvent{Text: "Some new "})
	tm.Send(domain.TextEvent{Text: "text without "})
	tm.Send(domain.TextEvent{Text: "flush..."})

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
	// Add some content (fragmented)
	tm.Send(domain.TextEvent{Text: "Some "})
	tm.Send(domain.TextEvent{Text: "text\n\n"})
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
	tm, m := newTeatestHarnessWithFrameLogAndCursor(t, 80, 24, 1)

	// Stream diverse content
	// Stream diverse content (Fragmented to simulate real streaming)
	tm.Send(domain.TextEvent{Text: "Hea"})
	tm.Send(domain.TextEvent{Text: "der\n"})
	tm.Send(domain.TextEvent{Text: "\nParag"})
	tm.Send(domain.TextEvent{Text: "raph 1\n\n"})
	tm.Send(domain.ToolStartEvent{CallID: "t1", ToolName: "test", Display: domain.StringDisplay("Running...")})
	tm.Send(domain.ToolEndEvent{CallID: "t1"})
	tm.Send(domain.DoneEvent{})

	// Wait for Done
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Done") && strings.Contains(string(b), "Context:")
	}, teatest.WithDuration(1*time.Second))

	// Analyze frames
	frames := m.GetFrameLog()
	for i, frame := range frames {
		// Skip empty or partial
		if strings.TrimSpace(frame) == "" {
			continue
		}

		lines := strings.Split(frame, "\n")
		// Find last occurrence of content and first of status bar
		// (Status bar is usually last few lines)
		contentRow := -1
		statusRow := -1

		for r, line := range lines {
			if strings.Contains(line, "Header") || strings.Contains(line, "Paragraph") {
				contentRow = r
			}
			// Identify status bar by "Context:" which is always present in the status bar
			// and never in content. This avoids false matches from content containing
			// "Done", "Generating", or "Thinking".
			if strings.Contains(line, "Context:") {
				if statusRow == -1 {
					statusRow = r
				}
			}
		}

		if contentRow != -1 && statusRow != -1 {
			if statusRow < contentRow {
				t.Errorf("Frame %d: Status bar (row %d) above content (row %d)!\nFrame:\n%s", i, statusRow, contentRow, frame)
			}
		}
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
			if strings.Contains(line, "Context:") {
				statusRow = r
				break
			}
		}

		if statusRow == -1 {
			// Status bar not found. Only fail if spinner is present (meaning status bar SHOULD be there).
			// If no spinner, it's likely an early partial render - acceptable.
			hasSpinner := strings.ContainsAny(frame, spinnerChars)
			if hasSpinner {
				t.Errorf("Frame %d: Spinner present but 'Context:' text missing from status bar", i)
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

// TestRegression_ContentIntegrity_NoDataLoss verifies all streamed chunks appear in final View frame.
func TestRegression_ContentIntegrity_NoDataLoss(t *testing.T) {
	tm, m := newTeatestHarnessWithFrameLog(t)

	chunks := []string{"Chunk 1 ", "Chunk 2 ", "Chunk 3"}
	for _, c := range chunks {
		tm.Send(domain.TextEvent{Text: c})
	}

	// Sync: ensure all chunks are processed
	tm.Send(domain.ThinkingEvent{})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Thinking")
	}, teatest.WithDuration(1*time.Second))

	frames := m.GetFrameLog()
	if len(frames) == 0 {
		t.Fatal("No frames captured")
	}
	lastFrame := frames[len(frames)-1]

	// Assert: All chunks must be visible in the View.
	// This confirms they were concatenated and rendered together.
	for _, c := range chunks {
		if !strings.Contains(lastFrame, c) {
			t.Errorf("Missing chunk: %q in final frame.\nFrame:\n%s", c, lastFrame)
		}
	}

	tm.Send(domain.DoneEvent{})
	readAllOutput(t, tm) // Flush and close
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
		// When a tool ends, it is flushed to history.
		// The View() may become "empty" of content (only padding + status bar remaining).
		// We should NOT fail on empty content.
		// However, we SHOULD check that the status bar is still present (or at least valid).
		// If the frame is completely empty string "", that might be valid if runState is Done,
		// but here runState should still be running.

		if strings.TrimSpace(frame) == "" {
			// If completely empty, check if we're done (not expected here yet)
			continue
		}

		// If not empty, it must contain status bar
		if !strings.Contains(frame, "Generating") && !strings.Contains(frame, "Thinking") && !strings.Contains(frame, "Context:") {
			t.Errorf("Frame %d post-end: Status bar missing! Content: %q", i, frame)
		}
	}

	tm.Send(domain.DoneEvent{})
	readAllOutput(t, tm)
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
	// Two paragraphs to ensure first one flushes. Fragmented.
	tm.Send(domain.TextEvent{Text: "Para "})
	tm.Send(domain.TextEvent{Text: "1.\n\n"})
	tm.Send(domain.TextEvent{Text: "Para "})
	tm.Send(domain.TextEvent{Text: "2 starts..."})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Para 2")
	}, teatest.WithDuration(1*time.Second))

	// Capture frames before Done
	frames := m.GetFrameLog()
	preDoneCount := len(frames)

	tm.Send(domain.DoneEvent{})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Done")
	}, teatest.WithDuration(1*time.Second))

	// Analyze frames *before* Done to ensuring no flash occurred during the flush.
	// We expect frames to contain content + status bar.
	if preDoneCount == 0 {
		t.Fatal("No frames captured before Done")
	}

	var lastStatusRow int = -1
	for i := 0; i < preDoneCount; i++ {
		frame := frames[i]
		if strings.TrimSpace(frame) == "" {
			continue
		}

		lines := strings.Split(frame, "\n")
		statusRow := -1
		for r, line := range lines {
			if strings.Contains(line, "Generating") || strings.Contains(line, "Thinking") || strings.Contains(line, "Context:") {
				statusRow = r
				break
			}
		}

		if statusRow != -1 {
			// Check stability
			if lastStatusRow != -1 && statusRow < lastStatusRow {
				t.Errorf("Frame %d pre-Done: Status bar jumped UP from row %d to %d (FLASH DETECTED)", i, lastStatusRow, statusRow)
			}
			lastStatusRow = statusRow
		} else {
			t.Errorf("Frame %d pre-Done: Status bar missing! Content: %q", i, frame)
		}
	}

	// We don't check post-Done frames because View() returns "" by design.
}

// TestRegression_MarkdownRendering verifies basic rendering of markdown elements.
func TestRegression_MarkdownRendering(t *testing.T) {
	tm, _ := newTeatestHarness(t)
	tm.Send(domain.TextEvent{Text: "# Hea"})
	tm.Send(domain.TextEvent{Text: "der 1\n\n"})
	tm.Send(domain.TextEvent{Text: "- It"})
	tm.Send(domain.TextEvent{Text: "em 1\n"})
	tm.Send(domain.TextEvent{Text: "- It"})
	tm.Send(domain.TextEvent{Text: "em 2\n\n"})
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

// TestRegression_NoDuplicateContent verifies content isn't duplicated in View.
func TestRegression_NoDuplicateContent(t *testing.T) {
	tm, m := newTeatestHarnessWithFrameLog(t)
	text := "Unique content string"
	// split into chunks
	tm.Send(domain.TextEvent{Text: "Unique "})
	tm.Send(domain.TextEvent{Text: "con"})
	tm.Send(domain.TextEvent{Text: "tent "})
	tm.Send(domain.TextEvent{Text: "string\n\n"})

	// Sync
	tm.Send(domain.ThinkingEvent{})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Thinking")
	}, teatest.WithDuration(1*time.Second))

	frames := m.GetFrameLog()
	if len(frames) == 0 {
		t.Fatal("No frames captured")
	}

	// Assert: Content should appear EXACTLY ONCE total.
	// It might be in View (pending) OR in History (flushed), but not both (duplicated).
	// We check the final output which includes history + final view.
	tm.Send(domain.DoneEvent{})
	output := readAllOutput(t, tm)

	count := strings.Count(output, text)
	if count != 1 {
		t.Errorf("Content %q appears %d times in output (expected 1). Output:\n%s", text, count, output)
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
	// Send 30 lines inside an open code block to force them to remain PENDING.
	// Fragmented sends to simulate streaming.
	tm.Send(domain.TextEvent{Text: "```\n"})
	for i := 0; i < 30; i++ {
		tm.Send(domain.TextEvent{Text: "Li"})
		tm.Send(domain.TextEvent{Text: "ne\n"})
	}

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

// TestRegression_MaxContentHeightTracking verifies that padding adjusts as content grows,
// keeping View height stable. Uses content inside an open code block to prevent flushing.
// If we used paragraph breaks (\n\n), content would flush to history, reducing maxContentHeight,
// which legitimately changes View height — that's a different test scenario.
func TestRegression_MaxContentHeightTracking(t *testing.T) {
	tm, m := newTeatestHarnessWithFrameLogAndCursor(t, 80, 24, 1)

	// Start with an open code block. This forces all subsequent lines to be pending.
	tm.Send(domain.TextEvent{Text: "```\n"})
	tm.Send(domain.TextEvent{Text: "Line 1\n"})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Line 1")
	}, teatest.WithDuration(1*time.Second))

	frames := m.GetFrameLog()
	if len(frames) == 0 {
		t.Fatal("No framesCaptured")
	}
	frame1 := frames[len(frames)-1]

	// Add many lines INSIDE the code block
	// Add many lines INSIDE the code block (fragmented)
	for i := 0; i < 10; i++ {
		tm.Send(domain.TextEvent{Text: fmt.Sprintf("New Line %d", i)})
		tm.Send(domain.TextEvent{Text: "\n"})
	}
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "New Line 9")
	}, teatest.WithDuration(1*time.Second))

	frames2 := m.GetFrameLog()
	if len(frames2) == 0 {
		t.Fatal("No framesCaptured 2")
	}
	frame2 := frames2[len(frames2)-1]

	// Check that View Height is roughly constant (meaning padding adjusted).
	// Using strings.Count(frame, "\n")
	height1 := strings.Count(frame1, "\n")
	height2 := strings.Count(frame2, "\n")

	// Allow small off-by-one difference due to timing/rendering artifacts
	if height2 < height1-1 || height2 > height1+1 {
		t.Errorf("View height unstable! Frame 1: %d, Frame 2: %d. Padding failed to compensate.", height1, height2)
	}

	// Close block and finish
	tm.Send(domain.TextEvent{Text: "```\n\n"})
	tm.Send(domain.DoneEvent{})
	readAllOutput(t, tm)
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
	// 2. Open code block (grows the pending area)
	// Fragmented for realism
	tm.Send(domain.TextEvent{Text: "```"})
	tm.Send(domain.TextEvent{Text: "go\n"})
	codeLines := []string{
		"package main\n",
		"func main() {\n",
		"    fmt.Println(\"Hello\")\n",
		"}\n",
	}
	for _, line := range codeLines {
		// Send line in halves
		half := len(line) / 2
		tm.Send(domain.TextEvent{Text: line[:half]})
		tm.Send(domain.TextEvent{Text: line[half:]})
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
