package runtime

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/Cyclone1070/iav/internal/ui/engine"
)

// msgPrintFinished is sent when a print command completes.
type msgPrintFinished struct{}

// Interpret converts an engine Effect into a tea.Cmd.
func Interpret(e engine.Effect) tea.Cmd {
	if e == nil {
		return nil
	}
	switch eff := e.(type) {
	case engine.PrintPayload:
		if eff.Raw {
			return tea.Printf("%s", eff.Content)
		}
		return tea.Sequence(
			tea.Println(eff.Content),
			func() tea.Msg { return msgPrintFinished{} },
		)
	case engine.QuitPayload:
		return tea.Quit
	default:
		return nil
	}
}
