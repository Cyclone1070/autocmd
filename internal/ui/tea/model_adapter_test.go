package tea

import (
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui/engine"
	"github.com/charmbracelet/bubbles/spinner"
)

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
			Markdown: md,
			Theme:    &mockTheme{},
			Layout:   &mockLayout{},
			Spinner:  &mockSpinner{},
			ViewTool: func(t *engine.ToolState) string {
				return string(t.Display.(domain.StringDisplay))
			},
		}
	}

	adapter := NewTeaModelAdapter(state, factory)
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

func (mockTheme) Success(s string) string  { return s }
func (mockTheme) Error(s string) string    { return s }
func (mockTheme) Muted(s string) string    { return s }
func (mockTheme) Primary(s string) string  { return s }
func (mockTheme) SpinnerStyle() string     { return "" }
func (mockTheme) Box(c string, w int, s engine.ToolStatus) string { return c }
func (mockTheme) Separator(w int, s engine.ToolStatus) string    { return "" }

type mockLayout struct{}

func (mockLayout) TruncateWithIndicator(content string, _ int) string {
	return content
}

type mockSpinner struct{}

func (mockSpinner) SpinnerView() string { return "⣾" }
