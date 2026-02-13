package compose

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui/engine"
	"github.com/Cyclone1070/iav/internal/ui/markdown"
	teapkg "github.com/Cyclone1070/iav/internal/ui/tea"
)

// staticCursorDetector for harness tests.
type staticCursorDetector struct {
	row int
}

func (d *staticCursorDetector) GetCursorRow() (int, error) {
	return d.row, nil
}

// --- New-path frame harness (engine + tea + markdown) ---

type harnessFramePhase string

const (
	phasePreUpdate  harnessFramePhase = "pre_update"
	phasePostUpdate harnessFramePhase = "post_update"
	phasePostCmd    harnessFramePhase = "post_cmd"
)

type harnessFrame struct {
	Phase             harnessFramePhase
	View              string
	TotalFlushedLines int
	MaxAbsoluteHeight int
	EventLabel        string
	QueueLen          int
	IsPrinting        bool
}

type harnessFrameHarness struct {
	adapter *teapkg.TeaModelAdapter
	frames  []harnessFrame
}

func newHarnessFrameHarness(t *testing.T, width, height, cursorRow int) *harnessFrameHarness {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.UI.ChatWindowWidth = width
	cd := &staticCursorDetector{row: cursorRow}
	geom, err := teapkg.ResolveGeometry(cfg, cd, height)
	if err != nil {
		t.Fatalf("ResolveGeometry: %v", err)
	}
	mdRenderer, err := markdown.NewGlamourRenderer(width)
	if err != nil {
		t.Fatalf("markdown.NewGlamourRenderer: %v", err)
	}
	sm := markdown.NewStream(mdRenderer)
	state := engine.NewInitialState(geom)
	factory := func(s *spinner.Model) engine.Deps {
		deps := NewEngineDeps(cfg, sm, width, func() string { return s.View() })
		deps.Spinner = nil
		return deps
	}
	adapter := teapkg.NewTeaModelAdapter(state, factory)
	return &harnessFrameHarness{adapter: adapter, frames: nil}
}

func (h *harnessFrameHarness) capture(phase harnessFramePhase, eventLabel string) {
	s := h.adapter.State
	h.frames = append(h.frames, harnessFrame{
		Phase:             phase,
		View:              h.adapter.View(),
		TotalFlushedLines: s.TotalFlushedLines,
		MaxAbsoluteHeight: s.MaxAbsoluteHeight,
		EventLabel:        eventLabel,
		QueueLen:          len(s.PrintQueue),
		IsPrinting:        s.IsPrinting,
	})
}

func (h *harnessFrameHarness) runCmdOnce(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

const maxHarnessCmdIterations = 100

func (h *harnessFrameHarness) ApplyEvent(ev domain.Event, eventLabel string) {
	h.capture(phasePreUpdate, eventLabel)
	_, cmd := h.adapter.Update(ev)
	h.capture(phasePostUpdate, eventLabel)
	iters := 0
	for cmd != nil && iters < maxHarnessCmdIterations {
		msg := h.runCmdOnce(cmd)
		h.capture(phasePostCmd, eventLabel)
		_, cmd = h.adapter.Update(msg)
		iters++
	}
}

func (h *harnessFrameHarness) Frames() []harnessFrame { return h.frames }

// --- Assertion helpers ---

func getStatusBarRow(frame string) int {
	lines := strings.Split(frame, "\n")
	for i, line := range lines {
		if strings.Contains(line, "Context:") {
			return i
		}
	}
	return -1
}

func getContentBottomRow(frame string) int {
	lines := strings.Split(frame, "\n")
	lastContent := -1
	for i, line := range lines {
		if strings.Contains(line, "Context:") {
			break
		}
		if strings.TrimSpace(line) != "" {
			lastContent = i
		}
	}
	return lastContent
}

type harnessAssertTB interface {
	Helper()
	Errorf(format string, args ...interface{})
}

func assertStatusBarNeverJumpsUp(t harnessAssertTB, frames []harnessFrame) {
	t.Helper()
	lastStatusRow := -1
	for i, f := range frames {
		row := getStatusBarRow(f.View)
		if row == -1 {
			continue
		}
		if lastStatusRow != -1 && row < lastStatusRow {
			t.Errorf("frame %d (%s): status bar jumped up from row %d to %d\nView:\n%s",
				i, f.Phase, lastStatusRow, row, f.View)
		}
		lastStatusRow = row
	}
}

func assertVisualBottomMonotonic(t harnessAssertTB, frames []harnessFrame) {
	t.Helper()
	lastVisualBottom := -1
	for i, f := range frames {
		lines := strings.Split(f.View, "\n")
		frameHeight := len(lines)
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			frameHeight--
		}
		if frameHeight == 0 {
			continue
		}
		visualBottom := f.TotalFlushedLines + frameHeight
		if lastVisualBottom != -1 && visualBottom < lastVisualBottom {
			t.Errorf("frame %d: visual bottom dropped %d -> %d (flush=%d, maxAbs=%d)\nView:\n%s",
				i, lastVisualBottom, visualBottom, f.TotalFlushedLines, f.MaxAbsoluteHeight, f.View)
		}
		if visualBottom > lastVisualBottom {
			lastVisualBottom = visualBottom
		}
	}
}

func assertMaxAbsoluteHeightMonotonic(t harnessAssertTB, frames []harnessFrame) {
	t.Helper()
	last := -1
	for i, f := range frames {
		if last != -1 && f.MaxAbsoluteHeight < last {
			t.Errorf("frame %d: MaxAbsoluteHeight decreased %d -> %d", i, last, f.MaxAbsoluteHeight)
		}
		if f.MaxAbsoluteHeight > last {
			last = f.MaxAbsoluteHeight
		}
	}
}

func assertStatusBarAfterContent(t harnessAssertTB, frames []harnessFrame) {
	t.Helper()
	for i, f := range frames {
		statusRow := getStatusBarRow(f.View)
		contentRow := getContentBottomRow(f.View)
		if statusRow != -1 && contentRow != -1 && statusRow < contentRow {
			t.Errorf("frame %d: status bar (row %d) above content (row %d)\nView:\n%s",
				i, statusRow, contentRow, f.View)
		}
	}
}

func assertNoFlushVisibilityGap(t harnessAssertTB, frames []harnessFrame, contentMustRemain []string) {
	t.Helper()
	for i, f := range frames {
		if f.Phase != phasePostUpdate || f.QueueLen == 0 {
			continue
		}
		for _, needle := range contentMustRemain {
			if !strings.Contains(f.View, needle) {
				t.Errorf("frame %d (post_update, queue=%d): content %q missing from view (flush visibility gap)\nView:\n%s",
					i, f.QueueLen, needle, f.View)
			}
		}
	}
}

// --- Invariant tests for new path ---

func TestNewPath_StatusBarNeverJumpsUp(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "A\n"}, "t1")
	h.ApplyEvent(domain.TextEvent{Text: "B\n\n"}, "t2")
	assertStatusBarNeverJumpsUp(t, h.Frames())
}

func TestNewPath_VisualBottomMonotonic(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "Line 1\n"}, "t1")
	h.ApplyEvent(domain.TextEvent{Text: "Line 2\n\n"}, "t2")
	h.ApplyEvent(domain.TextEvent{Text: "Line 3\n"}, "t3")
	assertVisualBottomMonotonic(t, h.Frames())
}

func TestNewPath_MaxAbsoluteHeightMonotonic(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "A\n"}, "t1")
	h.ApplyEvent(domain.TextEvent{Text: "B\n\n"}, "t2")
	assertMaxAbsoluteHeightMonotonic(t, h.Frames())
}

func TestNewPath_StatusBarAfterContent(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "A\n"}, "t1")
	h.ApplyEvent(domain.TextEvent{Text: "B\n\n"}, "t2")
	assertStatusBarAfterContent(t, h.Frames())
}

func TestNewPath_NoFlushVisibilityGap(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "Pre\n\n"}, "pre")
	h.ApplyEvent(domain.ToolStartEvent{CallID: "t1", ToolName: "x", Display: domain.StringDisplay("Running")}, "tool")
	h.ApplyEvent(domain.ToolEndEvent{CallID: "t1"}, "end")
	assertNoFlushVisibilityGap(t, h.Frames(), []string{"Pre"})
}

func TestNewPath_OrderingParity(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "Intro text\n\n"}, "intro")
	h.ApplyEvent(domain.ToolStartEvent{CallID: "a", ToolName: "x", Display: domain.StringDisplay("Tool A")}, "toolA")
	h.ApplyEvent(domain.ToolStartEvent{CallID: "b", ToolName: "y", Display: domain.StringDisplay("Tool B")}, "toolB")
	h.ApplyEvent(domain.ToolEndEvent{CallID: "a"}, "endA")
	h.ApplyEvent(domain.ToolEndEvent{CallID: "b"}, "endB")
	h.ApplyEvent(domain.TextEvent{Text: "trailer\n\n"}, "trailer")
	frames := h.Frames()
	var lastView string
	for i := len(frames) - 1; i >= 0; i-- {
		if frames[i].View != "" {
			lastView = frames[i].View
			break
		}
	}
	idxIntro := strings.Index(lastView, "Intro text")
	idxA := strings.Index(lastView, "Tool A")
	idxB := strings.Index(lastView, "Tool B")
	idxStatus := strings.Index(lastView, "Context:")
	if idxIntro == -1 || idxA == -1 || idxB == -1 || idxStatus == -1 {
		t.Fatalf("missing expected content in final view:\n%s", lastView)
	}
	if idxIntro > idxA {
		t.Error("parity: intro text must appear before Tool A")
	}
	if idxA > idxB {
		t.Error("parity: Tool A must appear before Tool B")
	}
	if idxB > idxStatus {
		t.Error("parity: status bar (Context:) must appear last")
	}
}

func TestNewPath_TextContentOrdering_Frames(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 1)
	h.ApplyEvent(domain.TextEvent{Text: "Intro text\n"}, "text")
	h.ApplyEvent(domain.TextEvent{Text: "More\n\n"}, "more")
	frames := h.Frames()
	if len(frames) < 2 {
		t.Errorf("expected at least 2 frames, got %d", len(frames))
	}
	assertStatusBarAfterContent(t, frames)
	assertVisualBottomMonotonic(t, frames)
}
