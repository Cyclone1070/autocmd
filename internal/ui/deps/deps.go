// Package deps builds engine.Deps from config and markdown stream.
// Lives in a separate package to avoid import cycles (compose <-> runtime).

package deps

import (
	"fmt"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/Cyclone1070/iav/internal/ui/engine"
	"github.com/Cyclone1070/iav/internal/ui/markdown"
)

// NewEngineDeps builds engine.Deps from config and markdown stream.
func NewEngineDeps(cfg *config.Config, sm *markdown.Stream, width int, getSpinnerView func() string) engine.Deps {
	th := ui.NewTheme(cfg.UI)
	shellHeight := cfg.UI.ShellOutputHeight
	return engine.Deps{
		Markdown: sm,
		Theme:    &themeAdapter{t: th},
		Layout:   layoutAdapter{},
		ViewTool: func(t *engine.ToolState) string {
			return viewTool(th, shellHeight, width, getSpinnerView, t)
		},
	}
}

type themeAdapter struct {
	t *ui.Theme
}

func (a *themeAdapter) Success(s string) string   { return a.t.Success(s) }
func (a *themeAdapter) Error(s string) string      { return a.t.Error(s) }
func (a *themeAdapter) Muted(s string) string      { return a.t.Muted(s) }
func (a *themeAdapter) Primary(s string) string    { return a.t.Primary(s) }
func (a *themeAdapter) SpinnerStyle() string       { return "" }

func (a *themeAdapter) Box(content string, width int, status engine.ToolStatus) string {
	return a.t.Box(content, width, toToolStatus(status))
}

func (a *themeAdapter) Separator(width int, status engine.ToolStatus) string {
	return a.t.Separator(width, toToolStatus(status))
}

func toToolStatus(s engine.ToolStatus) ui.ToolStatus {
	switch s {
	case engine.StatusRunning:
		return ui.StatusRunning
	case engine.StatusSuccess:
		return ui.StatusSuccess
	case engine.StatusError:
		return ui.StatusError
	default:
		return ui.StatusRunning
	}
}

type layoutAdapter struct{}

func (layoutAdapter) TruncateWithIndicator(content string, termHeight int) string {
	return ui.TruncateWithIndicator(content, termHeight)
}

func viewTool(th *ui.Theme, shellHeight, width int, getSpinnerView func() string, t *engine.ToolState) string {
	status := toToolStatus(t.Status)
	var prefix string
	switch status {
	case ui.StatusRunning:
		prefix = getSpinnerView()
	case ui.StatusSuccess:
		prefix = th.Success("✓")
	case ui.StatusError:
		prefix = th.Error("✗")
	}
	contentWidth := width - 2
	var content string
	switch d := t.Display.(type) {
	case domain.StringDisplay:
		content = ui.RenderString(th, d, status, t.Err, prefix)
	case domain.DiffDisplay:
		content = ui.RenderDiff(contentWidth, th, d, status, t.Err, prefix)
	case domain.ShellDisplay:
		content = ui.RenderShell(contentWidth, shellHeight, th, d, t.ShellOutput, status, t.Err, prefix)
	default:
		content = ui.Pad(fmt.Sprintf("Unknown display type: %T", d), prefix)
	}
	return th.Box(content, contentWidth, status)
}
