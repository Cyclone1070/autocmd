package ui

import (
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
)

// TestPaddingLogic verifies the padding behavior in various scenarios.
func TestPaddingLogic(t *testing.T) {
	cfg := config.DefaultConfig()
	// Disable animations/spinners for deterministic output

	m, err := newModel(cfg, mockCursorDetector{row: 20})
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}
	m.Init() // Ensure init runs

	t.Run("Initial State", func(t *testing.T) {
		view := m.View()
		if view == "" {
			// Current view returns valid string even if empty?
			// Line 364: return padding + statusBar
		}
		if !strings.Contains(view, "Context:") {
			t.Errorf("Expected status bar in initial view")
		}
	})

	t.Run("Text Flush causes unintended padding", func(t *testing.T) {
		// 1. Simulate text arrival explicitly using Lists to force newlines
		// "Item 1"
		// "Item 2"
		// "Item 3"
		msg1 := domain.TextEvent{Text: "* Item 1\n* Item 2\n* Item 3\n"}
		m.Update(msg{Event: msg1})

		// Render once to update maxContentHeight (logic is in View)
		m.View()
		t.Logf("Initial Max Height (List): %d", m.maxContentHeight)
		initialMax := m.maxContentHeight

		// Check internal state: maxContentHeight should be >= 3 (List items are distinct lines)
		if m.maxContentHeight < 3 {
			t.Errorf("Expected maxContentHeight >= 3, got %d", m.maxContentHeight)
		}

		// 2. Simulate text flush by starting a new block (Paragraph)
		msg2 := domain.TextEvent{Text: "\n\nNew Block"}
		// This should flush the List. "New Block" remains pending.

		cmds, _ := m.Update(msg{Event: msg2})
		_ = cmds

		// Log Max Height AFTER Flush but BEFORE View (intermediate state check not possible cleanly without internal inspection)
		// But we can check it now.
		t.Logf("Max Height After Flush (Reduced?): %d", m.maxContentHeight)

		// NOW: Content is flushed.
		view := m.View()

		t.Logf("Max Height After View (New Block): %d", m.maxContentHeight)

		// If bug exists, View will have ~3 lines of padding.
		// + lines for status bar.

		lines := strings.Count(view, "\n")
		// Status bar is ~1 line.
		// If padding is 3, total lines ~4-5.

		// If fixed, maxContentHeight should have reduced, so padding is 0.
		// View should assume new content is starting at the top?
		// Wait, if we flushed, the cursor in terminal moved down.
		// So we are drawing at the new bottom.
		// We WANT maxContentHeight to reset to 0 relative to the new cursor!

		if lines > 3 {
			t.Errorf("Detected potential unwanted padding. View has %d lines.", lines)
			// This confirms the "Text Flush" issue likely exists.
		}

		// STRICT CHECK: maxContentHeight should have reduced!
		// Initial was 23 (anchored).
		// We flushed 4 lines (3 items + newline).
		// Expect reduction by 4 => 19.
		// Allow some tolerance for internal representation but it MUST be less than initial.
		if m.maxContentHeight >= initialMax {
			t.Errorf("Bug confirmed: maxContentHeight did not reduce after flush. Initial: %d, Now: %d", initialMax, m.maxContentHeight)
		}
		t.Logf("Verified reduction: %d -> %d", initialMax, m.maxContentHeight)

	})
	t.Run("Safe Exit State Machine (Serial Queue)", func(t *testing.T) {
		m, _ := newModel(cfg, mockCursorDetector{row: 20})
		m.Init()

		// 1. Start a tool
		m.Update(msg{Event: domain.ToolStartEvent{CallID: "t1", ToolName: "test"}})

		// Helper to check state
		getPending := func() int {
			c := len(m.printQueue)
			if m.isPrinting {
				c++
			}
			return c
		}

		// 2. End the tool
		// This should queue the tool output and immediately start printing it.
		// Queue: [], Printing: Tool
		_, cmd := m.Update(msg{Event: domain.ToolEndEvent{CallID: "t1"}})
		if cmd == nil {
			t.Errorf("Expected Tool Print command (ProcessQueue), got nil")
		}
		if p := getPending(); p != 1 {
			t.Errorf("Expected pending=1 (Printing Tool), got %d", p)
		}

		// 3. Send Done (Simulate User Interrupt/Completion)
		// This should queue the Status Bar.
		// Since we are busy printing Tool, it should NOT start printing Status yet.
		// Queue: [Status], Printing: Tool
		_, cmd = m.Update(msg{Event: domain.DoneEvent{}})
		if cmd != nil {
			// Serial Queue logic: processQueue returns nil if isPrinting is true.
			// So handleDoneEvent -> Sequence(nil) -> nil.
			t.Errorf("Expected deferral (nil cmd because busy printing), got %v", cmd)
		}
		if p := getPending(); p != 2 {
			t.Errorf("Expected pending=2 (Printing Tool + Queued Status), got %d", p)
		}
		if m.runState != stateDone {
			t.Errorf("Expected runState=stateDone, got %v", m.runState)
		}

		// 4. MsgPrintFinished (Tool done)
		// This should trigger the next item in Queue (Status Bar).
		// Queue: [], Printing: Status
		_, cmd = m.Update(msgPrintFinished{})
		if cmd == nil {
			t.Errorf("Expected Status Print command (ProcessQueue), got nil")
		}
		if p := getPending(); p != 1 {
			t.Errorf("Expected pending=1 (Printing Status), got %d", p)
		}

		// 5. MsgPrintFinished (Status done)
		// Queue: [], Printing: None. State: Done. -> Should Quit.
		_, cmd = m.Update(msgPrintFinished{})

		// Verify final Quit
		if cmd == nil {
			t.Errorf("Expected Quit command, got nil")
		}
		if p := getPending(); p != 0 {
			t.Errorf("Expected pending=0, got %d", p)
		}
	})
}
