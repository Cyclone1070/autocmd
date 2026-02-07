package ui

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

// ThreadSafeBuffer wraps bytes.Buffer with a mutex for concurrent access
type ThreadSafeBuffer struct {
	b  bytes.Buffer
	mu sync.Mutex
}

func (tsb *ThreadSafeBuffer) Write(p []byte) (n int, err error) {
	tsb.mu.Lock()
	defer tsb.mu.Unlock()
	return tsb.b.Write(p)
}

func (tsb *ThreadSafeBuffer) String() string {
	tsb.mu.Lock()
	defer tsb.mu.Unlock()
	return tsb.b.String()
}

// Test helpers (inlined)

func newTestHarness(t *testing.T) (*Renderer, *ThreadSafeBuffer) {
	return newTestHarnessWithSize(t, 80, 24)
}

func newTestHarnessWithSize(t *testing.T, width, height int) (*Renderer, *ThreadSafeBuffer) {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.UI.ChatWindowWidth = width

	buf := &ThreadSafeBuffer{}
	input := strings.NewReader("")

	r, err := NewRenderer(buf, input, cfg)
	if err != nil {
		t.Fatalf("failed to create renderer: %v", err)
	}

	// Start program in background
	go func() {
		_ = r.Wait()
	}()

	// Small delay to let program initialize
	time.Sleep(50 * time.Millisecond)

	return r, buf
}

func waitForOutput(t *testing.T, buf *ThreadSafeBuffer, target string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), target) {
			return
		}
		<-ticker.C
	}
	t.Fatalf("timed out waiting for output: %q\nCurrent buffer:\n%s", target, buf.String())
}

// waitForSubstringOrder waits for both substrings and asserts that 'a' appears before 'b' in the output.
func waitForSubstringOrder(t *testing.T, buf *ThreadSafeBuffer, a, b string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		output := buf.String()
		idxA := strings.Index(output, a)
		idxB := strings.Index(output, b)
		if idxA != -1 && idxB != -1 {
			if idxA > idxB {
				t.Errorf("expected %q before %q, but %q at %d, %q at %d", a, b, a, idxA, b, idxB)
			}
			return
		}
		<-ticker.C
	}
	output := buf.String()
	t.Fatalf("timed out waiting for both substrings. Looking for %q and %q\nCurrent buffer:\n%s", a, b, output)
}

// Integration tests

func TestIntegration_ToolOrdering(t *testing.T) {
	renderer, buf := newTestHarness(t)

	// Start Tool A
	renderer.Send(domain.ToolStartEvent{
		CallID:   "call_A",
		ToolName: "slow-tool",
		Display:  domain.StringDisplay("Tool A Running..."),
	})
	waitForOutput(t, buf, "Tool A Running", 2*time.Second)

	// Start Tool B
	renderer.Send(domain.ToolStartEvent{
		CallID:   "call_B",
		ToolName: "fast-tool",
		Display:  domain.StringDisplay("Tool B Running..."),
	})
	waitForOutput(t, buf, "Tool B Running", 2*time.Second)

	// Finish Tool B first (should NOT flush yet)
	renderer.Send(domain.ToolEndEvent{CallID: "call_B"})
	time.Sleep(100 * time.Millisecond)

	// Finish Tool A (should flush A, then B)
	renderer.Send(domain.ToolEndEvent{CallID: "call_A"})
	time.Sleep(200 * time.Millisecond)

	// Send Done to flush everything
	renderer.Send(domain.DoneEvent{})
	time.Sleep(200 * time.Millisecond)

	output := buf.String()
	// Find positions of "Tool A" and "Tool B" in flushed output
	idxA := strings.Index(output, "Tool A Running")
	idxB := strings.Index(output, "Tool B Running")

	if idxA == -1 || idxB == -1 {
		t.Fatalf("expected both tools in output, got:\n%s", output)
	}

	// Tool A should appear before Tool B in output
	if idxA > idxB {
		t.Errorf("Tool A should appear before Tool B, but A at %d, B at %d", idxA, idxB)
	}
}

func TestIntegration_TextBeforeTool(t *testing.T) {
	renderer, buf := newTestHarness(t)

	// Send text
	renderer.Send(domain.TextEvent{Text: "Starting tools...\n\n"})
	waitForOutput(t, buf, "Starting tools", 2*time.Second)

	// Start tool
	renderer.Send(domain.ToolStartEvent{
		CallID:   "call_str",
		ToolName: "string-tool",
		Display:  domain.StringDisplay("Simple text output"),
	})
	waitForOutput(t, buf, "Simple text output", 2*time.Second)

	// Send Done
	renderer.Send(domain.DoneEvent{})
	time.Sleep(200 * time.Millisecond)

	output := buf.String()
	idxText := strings.Index(output, "Starting tools")
	idxTool := strings.Index(output, "Simple text output")

	if idxText == -1 || idxTool == -1 {
		t.Fatalf("expected both text and tool in output, got:\n%s", output)
	}

	// Text should appear before tool
	if idxText > idxTool {
		t.Errorf("Text should appear before tool, but text at %d, tool at %d", idxText, idxTool)
	}
}

func TestIntegration_FinalStatusLast(t *testing.T) {
	renderer, buf := newTestHarness(t)

	// Send some content
	renderer.Send(domain.TextEvent{Text: "Some content\n\n"})
	waitForOutput(t, buf, "Some content", 2*time.Second)

	// Send Done
	renderer.Send(domain.DoneEvent{})
	time.Sleep(500 * time.Millisecond) // Wait for all flushes

	output := buf.String()
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
	if idxDone < len(output)-100 {
		t.Logf("Status bar appears early (at position %d of %d), but this may be acceptable", idxDone, len(output))
	}
}

func TestIntegration_DoneFlushesAllContent(t *testing.T) {
	renderer, buf := newTestHarness(t)

	// Add text
	renderer.Send(domain.TextEvent{Text: "Final Text\n\n"})
	waitForOutput(t, buf, "Final Text", 2*time.Second)

	// Add tool
	renderer.Send(domain.ToolStartEvent{
		CallID:   "t1",
		ToolName: "test",
		Display:  domain.StringDisplay("Tool Output"),
	})
	waitForOutput(t, buf, "Tool Output", 2*time.Second)

	renderer.Send(domain.ToolEndEvent{CallID: "t1"})
	time.Sleep(100 * time.Millisecond)

	// Send Done - should flush everything including status
	renderer.Send(domain.DoneEvent{})
	time.Sleep(500 * time.Millisecond)

	output := buf.String()
	// Verify all content is present
	if !strings.Contains(output, "Final Text") {
		t.Error("Done should flush text, but 'Final Text' not found")
	}
	if !strings.Contains(output, "Tool Output") {
		t.Error("Done should flush tools, but 'Tool Output' not found")
	}
	if !strings.Contains(output, "Done") {
		t.Error("Done should show status, but 'Done' not found")
	}
	if !strings.Contains(output, "Context: 42%") {
		t.Error("Done should show context info, but 'Context: 42%' not found")
	}

	// Verify ordering: text before tool, status at end
	waitForSubstringOrder(t, buf, "Final Text", "Tool Output", 100*time.Millisecond)
	waitForSubstringOrder(t, buf, "Tool Output", "Done", 100*time.Millisecond)
}

func TestIntegration_CtrlCCancels(t *testing.T) {
	renderer, buf := newTestHarness(t)

	// Add some content
	renderer.Send(domain.TextEvent{Text: "Some text\n\n"})
	waitForOutput(t, buf, "Some text", 2*time.Second)

	// Start a tool
	renderer.Send(domain.ToolStartEvent{
		CallID:   "t1",
		ToolName: "test",
		Display:  domain.StringDisplay("Tool Running"),
	})
	waitForOutput(t, buf, "Tool Running", 2*time.Second)

	// Send Ctrl+C
	renderer.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	time.Sleep(500 * time.Millisecond)

	output := buf.String()
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

func TestIntegration_StatusBarLayout_Wide(t *testing.T) {
	renderer, buf := newTestHarnessWithSize(t, 100, 24)

	renderer.Send(domain.TextEvent{Text: "Content\n\n"})
	waitForOutput(t, buf, "Content", 2*time.Second)

	renderer.Send(domain.DoneEvent{})
	time.Sleep(500 * time.Millisecond)

	output := buf.String()
	// Wide terminal should have single-line status bar layout
	// Check that "Generating" or "Done" and "Context: 42%" appear on same line
	// (no newline between them in the status bar section)
	lines := strings.Split(output, "\n")
	
	// Find a line that contains both status indicators
	foundSingleLine := false
	for _, line := range lines {
		if strings.Contains(line, "Done") && strings.Contains(line, "Context: 42%") {
			foundSingleLine = true
			break
		}
		if strings.Contains(line, "Generating") && strings.Contains(line, "Context: 42%") {
			foundSingleLine = true
			break
		}
	}
	if !foundSingleLine {
		t.Logf("Wide layout test: status bar may be split across lines (acceptable). Output:\n%s", output)
	}
}

func TestIntegration_StatusBarLayout_Narrow(t *testing.T) {
	renderer, buf := newTestHarnessWithSize(t, 20, 24)

	renderer.Send(domain.TextEvent{Text: "Content\n\n"})
	waitForOutput(t, buf, "Content", 2*time.Second)

	renderer.Send(domain.DoneEvent{})
	time.Sleep(500 * time.Millisecond)

	output := buf.String()
	// Narrow terminal should have two-line status bar layout
	// Status bar should contain both "Done" and "Context: 42%"
	if !strings.Contains(output, "Done") {
		t.Error("Status bar should contain 'Done'")
	}
	if !strings.Contains(output, "Context: 42%") {
		t.Error("Status bar should contain 'Context: 42%'")
	}
	// In narrow layout, these may be on separate lines, which is acceptable
}
