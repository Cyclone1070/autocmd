package prompt

import (
	"fmt"
	"time"

	"github.com/Cyclone1070/iav/internal/ui"
)

// thinkingRenderer handles the "Thinking" state rendering.
type ThinkingRenderer struct {
	Theme *ui.Theme
}

// NewThinkingRenderer creates a new ThinkingRenderer.
func NewThinkingRenderer(th *ui.Theme) *ThinkingRenderer {
	return &ThinkingRenderer{Theme: th}
}

// RenderThinking renders a blank line above, status indicator, and label.
func (r *ThinkingRenderer) RenderThinking(status ui.ToolStatus, start time.Time, tick int, sp spinnerProvider) string {
	dur := time.Since(start).Round(time.Second)

	prefix := r.Theme.StatusPrefix(status, sp.Frame(tick))
	var label string

	switch status {
	case ui.StatusRunning:
		label = r.Theme.Primary(fmt.Sprintf("Thinking for %s", dur))
	case ui.StatusSuccess:
		label = r.Theme.Success(fmt.Sprintf("Thought for %s", dur))
	case ui.StatusError:
		label = r.Theme.Error(fmt.Sprintf("Thought for %s", dur))
	}

	return fmt.Sprintf("\n %s%s", prefix, label)
}
