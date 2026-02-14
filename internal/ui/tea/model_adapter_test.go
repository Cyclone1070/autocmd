package tea

import (
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui/engine"
	"github.com/charmbracelet/bubbles/spinner"
)

func TestNewTeaModelAdapter_PanicsOnNilSink(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when sink is nil")
		}
	}()
	geom := engine.Geometry{Width: 80, TermHeight: 24, SpaceBelow: 20}
	state := engine.NewInitialState(geom)
	factory := func(_ *spinner.Model) engine.Deps { return engine.Deps{} }
	_ = NewTeaModelAdapter(state, factory, nil)
}

func TestToEngineMsg_DomainEvents(t *testing.T) {
	tests := []struct {
		name   string
		teaMsg interface{}
		want   bool
	}{
		{"Thinking", domain.ThinkingEvent{}, true},
		{"Text", domain.TextEvent{Text: "hi"}, true},
		{"Done", domain.DoneEvent{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := toEngineMsg(tt.teaMsg)
			if ok != tt.want {
				t.Errorf("toEngineMsg() ok = %v, want %v", ok, tt.want)
			}
		})
	}
}

func TestAdapter_View_UsesEngineRender(t *testing.T) {
	geom := engine.Geometry{Width: 80, TermHeight: 24, SpaceBelow: 20}
	state := engine.NewInitialState(geom)
	state.MaxAbsoluteHeight = 20

	md := &mockMarkdownStream{}
	factory := func(_ *spinner.Model) engine.Deps {
		return engine.Deps{
			Markdown:     md,
			Theme:        &mockTheme{},
			Layout:       &mockLayout{},
			ToolRenderer: &mockToolRenderer{},
			Spinner:      &mockSpinner{},
		}
	}

	adapter := NewTeaModelAdapter(state, factory, NoopSink{})
	out := adapter.View()

	if !strings.Contains(out, "Context:") {
		t.Errorf("View should contain status bar, got %q", out)
	}
}

type mockMarkdownStream struct {
	buf string
}

func (m *mockMarkdownStream) Append(chunk string) ([]string, error) {
	m.buf += chunk
	return nil, nil
}

func (m *mockMarkdownStream) Pending() string {
	return strings.TrimRight(m.buf, "\n")
}

func (m *mockMarkdownStream) RenderRemaining() (string, error) {
	return strings.TrimRight(m.buf, "\n"), nil
}

type mockTheme struct{}

func (mockTheme) Success(s string) string                         { return s }
func (mockTheme) Error(s string) string                           { return s }
func (mockTheme) Muted(s string) string                           { return s }
func (mockTheme) Primary(s string) string                         { return s }
func (mockTheme) SpinnerStyle() string                            { return "" }
func (mockTheme) Box(c string, w int, s engine.ToolStatus) string { return c }
func (mockTheme) Separator(w int, s engine.ToolStatus) string     { return "" }

type mockLayout struct{}

func (mockLayout) TruncateWithIndicator(content string, _ int) string {
	return content
}

type mockSpinner struct{}

func (mockSpinner) SpinnerView() string { return "⣾" }

type mockToolRenderer struct{}

func (m *mockToolRenderer) Render(t *engine.ToolState, spinner engine.SpinnerViewProvider) string {
	if spinner != nil {
		return "Mock: " + spinner.SpinnerView() + " " + string(t.Display.(domain.StringDisplay))
	}
	return "Mock: " + string(t.Display.(domain.StringDisplay))
}

// TestSpinnerAnimatesInRunningTool verifies that when a tool is in Running state
// and spinner ticks occur, the rendered view changes (spinner animates).
// This test validates the FIX: spinner parameter is used at render time, not captured at creation time.
func TestSpinnerAnimatesInRunningTool(t *testing.T) {
	geom := engine.Geometry{Width: 80, TermHeight: 24, SpaceBelow: 20}
	state := engine.NewInitialState(geom)

	// Add a tool in Running state
	state.Tools = append(state.Tools, &engine.ToolState{
		CallID:  "test-tool-1",
		Display: domain.StringDisplay("Reading file..."),
		Status:  engine.StatusRunning,
		Err:     "",
	})

	// Create deps factory with FIXED toolRenderer (uses passed parameter)
	factory := func(s *spinner.Model) engine.Deps {
		return engine.Deps{
			Markdown:     &mockMarkdownStream{},
			Theme:        &mockTheme{},
			Layout:       &mockLayout{},
			Spinner:      nil, // Will be set at runtime
			ToolRenderer: &fixedToolRenderer{},
		}
	}

	sink := &RecordingSink{Events: []FrameEvent{}}
	adapter := NewTeaModelAdapter(state, factory, sink)

	// Capture initial view
	view1 := adapter.View()
	if !strings.Contains(view1, "Reading file") {
		t.Fatalf("Tool display not found in initial view")
	}

	// Extract spinner frames from the views
	extractSpinnerFrame := func(view string) string {
		// Extract the spinner character from "Spinner: X | ..."
		parts := strings.Split(view, " | ")
		if len(parts) < 2 {
			return ""
		}
		spinnerPart := strings.TrimPrefix(parts[0], "Spinner: ")
		return spinnerPart
	}

	frame1 := extractSpinnerFrame(view1)
	if frame1 == "" {
		t.Fatalf("Could not extract spinner frame from initial view: %s", view1)
	}

	// Send multiple spinner ticks to animate
	for i := 0; i < 5; i++ {
		tickMsg := spinner.TickMsg{}
		adapter.Update(tickMsg)
	}

	// Capture view after ticks
	view2 := adapter.View()
	frame2 := extractSpinnerFrame(view2)
	if frame2 == "" {
		t.Fatalf("Could not extract spinner frame from final view: %s", view2)
	}

	// ASSERTION: Spinner frames should be different (spinner animated)
	if frame1 == frame2 {
		t.Errorf("Spinner did not animate. Frame remained identical after 5 ticks.\nFrame 1: %q\nFrame 2: %q",
			frame1, frame2)
	}
}

// fixedToolRenderer implements engine.ToolRenderer: uses spinner parameter instead of capturing
type fixedToolRenderer struct{}

func (r *fixedToolRenderer) Render(t *engine.ToolState, spinner engine.SpinnerViewProvider) string {
	if t.Status == engine.StatusRunning {
		// FIX: Using the parameter 'spinner' which is updated at runtime!
		spinnerView := ""
		if spinner != nil {
			spinnerView = spinner.SpinnerView()
		}
		return "Spinner: " + spinnerView + " | " + string(t.Display.(domain.StringDisplay))
	}
	return string(t.Display.(domain.StringDisplay))
}
