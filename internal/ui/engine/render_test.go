package engine

import (
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui/theme"
)

func TestRender_EmptyState_HasActivityIndicator(t *testing.T) {
	geom := TermSize{Width: 80, Height: 24}
	state := NewInitialState(geom)
	deps := testDeps(&mockMarkdown{})

	out := Render(state, deps)

	if !strings.Contains(out, ".") {
		t.Errorf("expected activity indicator, got %q", out)
	}
}

func TestRender_Done_ReturnsEmpty(t *testing.T) {
	geom := TermSize{Width: 80, Height: 24}
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
	geom := TermSize{Width: 80, Height: 24}
	state := NewInitialState(geom)
	deps := testDeps(md)

	out := Render(state, deps)

	if !strings.Contains(out, "Hello world") {
		t.Errorf("expected pending content, got %q", out)
	}
}

func TestRender_WithTool(t *testing.T) {
	geom := TermSize{Width: 80, Height: 24}
	state := NewInitialState(geom)
	state.Tools = []*ToolState{
		{CallID: "t1", Display: domain.StringDisplay("Tool output"), Status: theme.StatusRunning},
	}
	deps := testDeps(&mockMarkdown{})

	out := Render(state, deps)

	if !strings.Contains(out, "Tool output") {
		t.Errorf("expected tool content, got %q", out)
	}
}
