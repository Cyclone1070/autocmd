package tea

import (
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui/engine"
	"github.com/Cyclone1070/iav/internal/ui/theme"
)

func TestNewTeaModelAdapter_PanicsOnNilSink(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when sink is nil")
		}
	}()
	geom := engine.TermSize{Width: 80, Height: 24}
	state := engine.NewInitialState(geom)
	factory := func() engine.Deps { return engine.Deps{} }
	_ = NewTeaModelAdapter(state, factory, nil)
}

func TestToEngineMsg_DomainEvents(t *testing.T) {
	tests := []struct {
		name   string
		teaMsg interface{}
		want   bool
	}{
		{"Tick", engine.MsgTick{}, true},
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
	geom := engine.TermSize{Width: 80, Height: 24}
	state := engine.NewInitialState(geom)

	md := &mockMarkdownStream{}
	factory := func() engine.Deps {
		return engine.Deps{
			Markdown:     md,
			Theme:        &mockTheme{},
			Layout:       &mockLayout{},
			ToolRenderer: &mockToolRenderer{},
		}
	}

	adapter := NewTeaModelAdapter(state, factory, NoopSink{})
	out := adapter.View()

	if !strings.Contains(out, ".") {
		t.Errorf("View should contain activity indicator, got %q", out)
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

func (mockTheme) Success(s string) string                        { return s }
func (mockTheme) Error(s string) string                          { return s }
func (mockTheme) Muted(s string) string                          { return s }
func (mockTheme) Primary(s string) string                        { return s }
func (mockTheme) Box(c string, w int, s theme.ToolStatus) string { return c }
func (mockTheme) Separator(w int, s theme.ToolStatus) string     { return "" }

type mockLayout struct{}

func (mockLayout) TruncateWithIndicator(content string, _ int) string {
	return content
}

type mockToolRenderer struct{}

func (m *mockToolRenderer) Render(t *engine.ToolState) string {
	return "Mock: " + string(t.Display.(domain.StringDisplay))
}
