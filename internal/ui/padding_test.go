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

	m, err := newModel(cfg)
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}
	m.Init() // Ensure init runs

	// Helper to get current padding count
	getPaddingLines := func(view string) int {
		// Placeholder for debugging
		return 0
	}
	_ = getPaddingLines

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
			t.Logf("Detected potential unwanted padding. View has %d lines.", lines)
			// This confirms the "Text Flush" issue likely exists.
		}

		// STRICT CHECK: maxContentHeight should have reduced!
		// Initial was 5. If bug exists, it stays 5.
		// It dropped to 0, then grew to 2 (new content height).
		// So we expect < 5.
		if m.maxContentHeight >= 5 {
			t.Errorf("Bug confirmed: maxContentHeight did not reset/reduce after flush. Value: %d", m.maxContentHeight)
		}

		// Clear lines usage for linter
		_ = lines
	})
}

// msg wrapper to match model.go's private msg type if needed,
// but model.Update takes tea.Msg.
// model.go: case msg: return m.handleEvent(ev.Event)
// We need to construct a 'msg' which is private?
// Ah, 'msg' struct line 102 is private.
// But we are in 'package ui', so we can access it!
// We just need to check if 'msg' is defined in model.go or helpers.go?
// It's likely in model.go or similar.
// A quick check of model.go showed: case msg:
// So yes, we can use it.
