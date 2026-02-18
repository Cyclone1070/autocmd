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
	switch e.(type) {
	case engine.PrintPayload, engine.QuitPayload:
		return nil // Handled by adapter via sink
	case interface{ isEffectScheduleTick() }: // internal check if we don't want to export the struct
		return bubbletea.Tick(100*time.Millisecond, func(t time.Time) bubbletea.Msg {
			return engine.MsgTick{}
		})
	default:
		return nil
	}
}
