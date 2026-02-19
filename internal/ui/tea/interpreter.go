package tea

import (
	"time"

	"github.com/Cyclone1070/iav/internal/ui/engine"
	bubbletea "github.com/charmbracelet/bubbletea"
)

// msgPrintFinished is sent when a print command completes.
type MsgPrintFinished struct{}

// Interpret converts an engine Effect into a tea.Cmd.
// PrintPayload and QuitPayload are handled by the adapter via FrameSink; this only handles other effects.
func Interpret(e engine.Effect) bubbletea.Cmd {
	if e == nil {
		return nil
	}
	if e.IsTick() {
		return bubbletea.Tick(16*time.Millisecond, func(t time.Time) bubbletea.Msg {
			return engine.MsgTick{}
		})
	}
	return nil
}
