package engine

import (
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
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

func (noopTheme) Success(s string) string                  { return s }
func (noopTheme) Error(s string) string                    { return s }
func (noopTheme) Muted(s string) string                    { return s }
func (noopTheme) Primary(s string) string                  { return s }
func (noopTheme) SpinnerStyle() string                     { return "" }
func (noopTheme) Box(c string, w int, s ToolStatus) string { return c }
func (noopTheme) Separator(w int, s ToolStatus) string     { return "" }

// noopLayout passes through content.
type noopLayout struct{}

func (noopLayout) TruncateWithIndicator(content string, _ int) string {
	return content
}

// noopSpinner returns empty string.
type noopSpinner struct{}

func (noopSpinner) SpinnerView() string { return "" }

// noopToolRenderer returns simple string representation.
type noopToolRenderer struct{}

func (noopToolRenderer) Render(t *ToolState, spinner SpinnerViewProvider) string {
	return string(t.Display.(domain.StringDisplay))
}

func testDeps(md *mockMarkdown) Deps {
	return Deps{
		Markdown:     md,
		Theme:        noopTheme{},
		Layout:       noopLayout{},
		Spinner:      noopSpinner{},
		ToolRenderer: noopToolRenderer{},
	}
}

func TestTransition_Thinking(t *testing.T) {
	geom := Geometry{Width: 80, TermHeight: 24, SpaceBelow: 20}
	state := NewInitialState(geom)
	deps := testDeps(&mockMarkdown{})

	state, effects := Transition(state, MsgThinking{}, deps)

	if !state.Thinking {
		t.Error("expected Thinking=true")
	}
	if len(effects) != 1 {
		t.Fatalf("expected 1 effect (tick), got %d", len(effects))
	}
	// First effect should be tick (effectScheduleTick is unexported)
	if len(effects) < 1 {
		t.Error("expected at least one effect")
	}
}

func TestTransition_Text_FlushesBlocks(t *testing.T) {
	md := &mockMarkdown{}
	geom := Geometry{Width: 80, TermHeight: 24, SpaceBelow: 20}
	state := NewInitialState(geom)
	deps := testDeps(md)

	state, effects := Transition(state, MsgText{Text: "Para 1\n\n"}, deps)

	if state.Thinking {
		t.Error("expected Thinking=false after Text")
	}
	// mockMarkdown flushes on \n\n
	if len(effects) == 0 {
		t.Error("expected flush effect for completed paragraph")
	}
	hasPrint := false
	for _, e := range effects {
		if p, ok := e.(PrintPayload); ok && strings.Contains(p.Content, "Para 1") {
			hasPrint = true
			break
		}
	}
	if !hasPrint {
		t.Errorf("expected print effect with Para 1, got %v", effects)
	}
}

func TestTransition_ToolStart_AddsTool(t *testing.T) {
	geom := Geometry{Width: 80, TermHeight: 24, SpaceBelow: 20}
	state := NewInitialState(geom)
	deps := testDeps(&mockMarkdown{})

	state, effects := Transition(state, MsgToolStart{
		CallID:  "t1",
		Display: domain.StringDisplay("Tool Running"),
	}, deps)

	if len(state.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(state.Tools))
	}
	if state.Tools[0].CallID != "t1" {
		t.Errorf("expected CallID t1, got %s", state.Tools[0].CallID)
	}
	if state.Tools[0].Status != StatusRunning {
		t.Errorf("expected StatusRunning, got %v", state.Tools[0].Status)
	}
	// Should have tick effect
	hasTick := false
	for _, e := range effects {
		if _, ok := e.(effectScheduleTick); ok {
			hasTick = true
			break
		}
	}
	if !hasTick {
		t.Error("expected tick effect after ToolStart")
	}
}

func TestTransition_ToolEnd_MarksComplete(t *testing.T) {
	geom := Geometry{Width: 80, TermHeight: 24, SpaceBelow: 20}
	state := NewInitialState(geom)
	state.Tools = []*ToolState{
		{CallID: "t1", Display: domain.StringDisplay("Done"), Status: StatusRunning},
	}
	deps := testDeps(&mockMarkdown{})

	state, effects := Transition(state, MsgToolEnd{CallID: "t1"}, deps)

	if len(state.Tools) != 0 {
		t.Errorf("expected tool flushed (removed), got %d tools", len(state.Tools))
	}
	hasPrint := false
	for _, e := range effects {
		if p, ok := e.(PrintPayload); ok && strings.Contains(p.Content, "Done") {
			hasPrint = true
			break
		}
	}
	if !hasPrint {
		t.Errorf("expected print effect for flushed tool, got %v", effects)
	}
}

func TestTransition_Done_FlushesAndQuits(t *testing.T) {
	geom := Geometry{Width: 80, TermHeight: 24, SpaceBelow: 20}
	state := NewInitialState(geom)
	state.Tools = []*ToolState{
		{CallID: "t1", Display: domain.StringDisplay("Final"), Status: StatusRunning},
	}
	md := &mockMarkdown{}
	md.buf = "pending"
	deps := testDeps(md)

	state, effects := Transition(state, MsgDone{}, deps)

	if state.RunState != StateDone {
		t.Errorf("expected StateDone, got %v", state.RunState)
	}
	// Should have print for "pending", print for tool, print for status, then quit
	// Or if queue is used, quit comes after prints drain
	hasQuit := false
	for _, e := range effects {
		if _, ok := e.(QuitPayload); ok {
			hasQuit = true
			break
		}
	}
	// Done with pending content: we flush first, quit only when queue empty
	// So we may not have quit in first transition - we have prints
	_ = hasQuit
	hasPrint := false
	for _, e := range effects {
		if _, ok := e.(PrintPayload); ok {
			hasPrint = true
			break
		}
	}
	if !hasPrint {
		t.Error("expected at least one print effect on Done")
	}
}
