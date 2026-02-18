package engine

import (
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui/theme"
)

// mockMarkdown appends and returns flushed on \n\n boundaries.
type mockMarkdown struct {
	buf string
}

func (m *mockMarkdown) Append(chunk string) ([]string, error) {
	m.buf += chunk
	var flushed []string
	if idx := strings.Index(m.buf, "\n\n"); idx != -1 {
		block := strings.TrimRight(m.buf[:idx+2], "\n")
		m.buf = m.buf[idx+2:]
		if block != "" {
			flushed = append(flushed, block)
		}
	}
	return flushed, nil
}

func (m *mockMarkdown) Pending() string {
	return strings.TrimRight(m.buf, "\n")
}

func (m *mockMarkdown) RenderRemaining() (string, error) {
	out := strings.TrimRight(m.buf, "\n")
	m.buf = ""
	return out, nil
}

// noopTheme implements ThemeAdapter with passthrough.
type noopTheme struct{}

func (noopTheme) Success(s string) string                        { return s }
func (noopTheme) Error(s string) string                          { return s }
func (noopTheme) Muted(s string) string                          { return s }
func (noopTheme) Primary(s string) string                        { return s }
func (noopTheme) Box(c string, w int, s theme.ToolStatus) string { return c }
func (noopTheme) Separator(w int, s theme.ToolStatus) string     { return "" }

// noopLayout passes through content.
type noopLayout struct{}

func (noopLayout) TruncateWithIndicator(content string, _ int) string {
	return content
}

// noopToolRenderer returns simple string representation.
type noopToolRenderer struct{}

func (noopToolRenderer) Render(t *ToolState) string {
	return string(t.Display.(domain.StringDisplay))
}

func testDeps(md *mockMarkdown) Deps {
	return Deps{
		Markdown:     md,
		Theme:        noopTheme{},
		Layout:       noopLayout{},
		ToolRenderer: noopToolRenderer{},
	}
}

func TestTransition_MsgTick_SimulatesTyping(t *testing.T) {
	md := &mockMarkdown{}
	geom := TermSize{Width: 80, Height: 24}
	state := NewInitialState(geom)
	state.TypingBuffer = "Hello world\n\n"
	deps := testDeps(md)

	// One tick should consume 4 chars (const charsPerTick = 4)
	state, effects := Transition(state, MsgTick{}, deps)

	if len(state.TypingBuffer) != 9 {
		t.Errorf("expected 9 chars left in buffer, got %d", len(state.TypingBuffer))
	}
	if state.IdleTicks != 0 {
		t.Error("IdleTicks should be reset while typing")
	}

	// Drain enough ticks until flushed
	for i := 0; i < 10; i++ {
		state, effects = Transition(state, MsgTick{}, deps)
		for _, e := range effects {
			if _, ok := e.(PrintPayload); ok {
				state, _ = Transition(state, MsgPrintFinished{}, deps)
			}
		}
		if state.TypingBuffer == "" && !state.IsPrinting && len(state.PrintQueue) == 0 {
			break
		}
	}

	hasPrint := false
	// check transcript or state
	if state.TotalFlushedLines > 0 {
		hasPrint = true
	}
	if !hasPrint {
		t.Error("expected print effect after typing buffer drains")
	}
}

func TestTransition_MsgText_AppendsToBuffer(t *testing.T) {
	geom := TermSize{Width: 80, Height: 24}
	state := NewInitialState(geom)
	deps := testDeps(&mockMarkdown{})

	state, _ = Transition(state, MsgText{Text: "para1"}, deps)
	if state.TypingBuffer != "para1" {
		t.Errorf("expected TypingBuffer 'para1', got %q", state.TypingBuffer)
	}
}

func TestTransition_ToolStart_FlushesMarkdown(t *testing.T) {
	md := &mockMarkdown{}
	md.buf = "pending"
	geom := TermSize{Width: 80, Height: 24}
	state := NewInitialState(geom)
	deps := testDeps(md)

	state, effects := Transition(state, MsgToolStart{
		CallID:  "t1",
		Display: domain.StringDisplay("Tool Running"),
	}, deps)

	hasPrint := false
	for _, e := range effects {
		if p, ok := e.(PrintPayload); ok && strings.Contains(p.Content, "pending") {
			hasPrint = true
			break
		}
	}
	if !hasPrint {
		t.Error("expected pending markdown to flush on ToolStart")
	}
}

func TestTransition_ToolEnd_FlushesTool(t *testing.T) {
	geom := TermSize{Width: 80, Height: 24}
	state := NewInitialState(geom)
	state.Tools = []*ToolState{
		{CallID: "t1", Display: domain.StringDisplay("Done"), Status: theme.StatusRunning},
	}
	deps := testDeps(&mockMarkdown{})

	// Mark it as done first
	state.Tools[0].Status = theme.StatusSuccess

	state, effects := Transition(state, MsgToolEnd{CallID: "t1"}, deps)

	if len(state.Tools) != 0 {
		t.Errorf("expected tool flushed, got %d tools", len(state.Tools))
	}
	hasPrint := false
	for _, e := range effects {
		if p, ok := e.(PrintPayload); ok && strings.Contains(p.Content, "Done") {
			hasPrint = true
			break
		}
	}
	if !hasPrint {
		t.Error("expected tool output to print on ToolEnd")
	}
}

func TestTransition_Done_FlushesEverything(t *testing.T) {
	geom := TermSize{Width: 80, Height: 24}
	state := NewInitialState(geom)
	state.TypingBuffer = "Final"
	state.Tools = []*ToolState{
		{CallID: "t1", Display: domain.StringDisplay("Tool"), Status: theme.StatusSuccess},
	}
	deps := testDeps(&mockMarkdown{})

	state, effects := Transition(state, MsgDone{}, deps)

	// Should have prints for TypingBuffer+Markdown, Tool, and Status
	printCount := 0
	for _, e := range effects {
		if _, ok := e.(PrintPayload); ok {
			printCount++
		}
	}
	// Note: startNextPrint only returns ONE effect at a time.
	// But handleDoneEvent calls enqueuePrint multiple times.
	// enqueuePrint calls startNextPrint.
	// If IsPrinting=false, startNextPrint returns EffectPrint.
	// Subsequent enqueuePrint calls will see IsPrinting=true and return nil.
	// So only the FIRST print effect is returned in effects slice directly from handleDoneEvent?
	// Wait, handleDoneEvent appends what enqueuePrint returns.
	// enqueuePrint returns startNextPrint(state).
	// If IsPrinting=false, it returns Effect.
	// Then IsPrinting=true.
	// Next enqueuePrint returns nil.
	// So yes, ONLY THE FIRST PRINT EFFECT is returned in the slice.
	// The rest stay in PrintQueue.
	if printCount != 1 {
		t.Errorf("expected 1 initial print effect, got %d", printCount)
	}
	if len(state.PrintQueue) < 2 {
		t.Errorf("expected at least 2 items in queue, got %d", len(state.PrintQueue))
	}
}
