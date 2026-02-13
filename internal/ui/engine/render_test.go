package engine

import (
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
)

func TestRender_EmptyState_HasStatusBar(t *testing.T) {
	geom := Geometry{Width: 80, TermHeight: 24, SpaceBelow: 20}
	state := NewInitialState(geom)
	state.MaxAbsoluteHeight = 20
	deps := testDeps(&mockMarkdown{})

	out := Render(state, deps)

	if !strings.Contains(out, "Context:") {
		t.Errorf("expected status bar with Context, got %q", out)
	}
	if !strings.Contains(out, "Generating") {
		t.Errorf("expected Generating in status, got %q", out)
	}
}

func TestRender_Done_ReturnsEmpty(t *testing.T) {
	geom := Geometry{Width: 80, TermHeight: 24, SpaceBelow: 20}
	state := NewInitialState(geom)
	state.RunState = StateDone
	deps := testDeps(&mockMarkdown{})

	out := Render(state, deps)

	if out != "" {
		t.Errorf("expected empty when Done, got %q", out)
	}
}

func TestRender_WithPendingContent(t *testing.T) {
	md := &mockMarkdown{}
	md.buf = "Hello world"
	geom := Geometry{Width: 80, TermHeight: 24, SpaceBelow: 20}
	state := NewInitialState(geom)
	state.MaxAbsoluteHeight = 25
	deps := testDeps(md)

	out := Render(state, deps)

	if !strings.Contains(out, "Hello world") {
		t.Errorf("expected pending content, got %q", out)
	}
	if !strings.Contains(out, "Context:") {
		t.Errorf("expected status bar, got %q", out)
	}
}

func TestRender_WithTool(t *testing.T) {
	geom := Geometry{Width: 80, TermHeight: 24, SpaceBelow: 20}
	state := NewInitialState(geom)
	state.Tools = []*ToolState{
		{CallID: "t1", Display: domain.StringDisplay("Tool output"), Status: StatusRunning},
	}
	state.MaxAbsoluteHeight = 25
	deps := testDeps(&mockMarkdown{})

	out := Render(state, deps)

	if !strings.Contains(out, "Tool output") {
		t.Errorf("expected tool content, got %q", out)
	}
}
