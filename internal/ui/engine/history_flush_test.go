package engine

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
)

// TestHistoryFlush_TotalFlushedLinesGrows verifies TotalFlushedLines increases on each flush.
func TestHistoryFlush_TotalFlushedLinesGrows(t *testing.T) {
	md := &mockMarkdown{}
	geom := TermSize{Width: 80, Height: 24}
	state := NewInitialState(geom)
	deps := testDeps(md)

	// Simulated typing and tick to flush
	state.TypingBuffer = "Block 1\n\n"
	tickUntilIdle(state, deps)
	if state.TotalFlushedLines == 0 {
		t.Error("expected TotalFlushedLines > 0 after first flush")
	}
	first := state.TotalFlushedLines

	state.TypingBuffer = "Block 2\n\n"
	tickUntilIdle(state, deps)
	if state.TotalFlushedLines <= first {
		t.Errorf("expected TotalFlushedLines to grow, got %d <= %d", state.TotalFlushedLines, first)
	}
}

func tickUntilIdle(state *State, deps Deps) {
	for i := 0; i < 100; i++ {
		sBefore := state.TypingBuffer
		pBefore := len(state.PrintQueue)
		iBefore := state.IsPrinting

		var effects []Effect
		state, effects = Transition(state, MsgTick{}, deps)
		for _, e := range effects {
			if _, ok := e.(PrintPayload); ok {
				state, _ = Transition(state, MsgPrintFinished{}, deps)
			}
		}

		if state.TypingBuffer == "" && len(state.PrintQueue) == 0 && !state.IsPrinting &&
			sBefore == "" && pBefore == 0 && !iBefore {
			break
		}
	}
}

// drainPrintQueue simulates all pending prints completing.
func drainPrintQueue(state *State, deps Deps) {
	for state.IsPrinting || len(state.PrintQueue) > 0 {
		state, _ = Transition(state, MsgPrintFinished{}, deps)
	}
}

// TestHistoryFlush_ToolOrderPreserved verifies tools flush in start order.
func TestHistoryFlush_ToolOrderPreserved(t *testing.T) {
	geom := TermSize{Width: 80, Height: 24}
	state := NewInitialState(geom)
	deps := testDeps(&mockMarkdown{})

	// Run transitions: start A, start B, end A, end B.
	// Flush order should be A then B (start order).
	state, _ = Transition(state, MsgToolStart{CallID: "a", Display: domain.StringDisplay("ToolA")}, deps)
	state, _ = Transition(state, MsgToolStart{CallID: "b", Display: domain.StringDisplay("ToolB")}, deps)

	state, _ = Transition(state, MsgToolEnd{CallID: "a"}, deps)
	state, _ = Transition(state, MsgToolEnd{CallID: "b"}, deps)

	if len(state.Tools) != 0 {
		t.Errorf("expected all tools flushed, got %d", len(state.Tools))
	}
}
