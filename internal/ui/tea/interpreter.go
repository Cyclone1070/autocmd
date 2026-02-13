package tea

import (
	"github.com/Cyclone1070/iav/internal/ui/engine"
	bubbletea "github.com/charmbracelet/bubbletea"
)

// msgPrintFinished is sent when a print command completes.
type msgPrintFinished struct{}

// Interpret converts an engine Effect into a tea.Cmd.
func Interpret(e engine.Effect) bubbletea.Cmd {
	if e == nil {
		return nil
	}
	switch eff := e.(type) {
	case engine.PrintPayload:
		if eff.Raw {
			return bubbletea.Printf("%s", eff.Content)
		}
		return bubbletea.Sequence(
			bubbletea.Println(eff.Content),
			func() bubbletea.Msg { return msgPrintFinished{} },
		)
	case engine.QuitPayload:
		return bubbletea.Quit
	default:
		return nil
	}
}
