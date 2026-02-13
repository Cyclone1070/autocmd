package tea

import (
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui/engine"
	bubbletea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/spinner"
)

// DepsFactory returns engine.Deps given the spinner (so ViewTool can use spinner.View()).
type DepsFactory func(spinner *spinner.Model) engine.Deps

// TeaModelAdapter adapts the engine to Bubble Tea's Model interface.
type TeaModelAdapter struct {
	State   *engine.State
	Deps    engine.Deps
	Spinner spinner.Model
}

// NewTeaModelAdapter creates an adapter with the given state and deps factory.
// The factory receives the spinner so ViewTool can render with the current spinner frame.
func NewTeaModelAdapter(state *engine.State, factory DepsFactory) *TeaModelAdapter {
	s := spinner.New()
	s.Spinner = spinner.Dot
	deps := factory(&s)
	return &TeaModelAdapter{
		State:   state,
		Deps:    deps,
		Spinner: s,
	}
}

// Init returns the initial command (spinner tick).
func (a *TeaModelAdapter) Init() bubbletea.Cmd {
	return a.Spinner.Tick
}

// Update processes messages and returns updated model and command.
func (a *TeaModelAdapter) Update(teaMsg bubbletea.Msg) (bubbletea.Model, bubbletea.Cmd) {
	deps := a.Deps
	deps.Spinner = &spinnerProvider{m: &a.Spinner}

	engineMsg, ok := toEngineMsg(teaMsg)
	if ok {
		_, effects := engine.Transition(a.State, engineMsg, deps)
		cmd := toTeaCmd(effects, a)
		return a, cmd
	}

	if _, ok := teaMsg.(spinner.TickMsg); ok {
		var cmd bubbletea.Cmd
		a.Spinner, cmd = a.Spinner.Update(teaMsg)
		return a, cmd
	}

	return a, nil
}

// View delegates to engine.Render.
func (a *TeaModelAdapter) View() string {
	deps := a.Deps
	deps.Spinner = &spinnerProvider{m: &a.Spinner}
	return engine.Render(a.State, deps)
}

type spinnerProvider struct {
	m *spinner.Model
}

func (s *spinnerProvider) SpinnerView() string {
	return s.m.View()
}

func toEngineMsg(teaMsg bubbletea.Msg) (engine.Msg, bool) {
	switch ev := teaMsg.(type) {
	case domain.ThinkingEvent:
		return engine.MsgThinking{}, true
	case domain.TextEvent:
		return engine.MsgText{Text: ev.Text}, true
	case domain.ToolStartEvent:
		return engine.MsgToolStart{CallID: ev.CallID, Display: ev.Display}, true
	case domain.ToolStreamEvent:
		return engine.MsgToolStream{CallID: ev.CallID, Chunk: ev.Chunk}, true
	case domain.ToolEndEvent:
		return engine.MsgToolEnd{CallID: ev.CallID, Error: ev.Error}, true
	case domain.DoneEvent:
		return engine.MsgDone{}, true
	case msgPrintFinished:
		return engine.MsgPrintFinished{}, true
	case bubbletea.KeyMsg:
		if ev.Type == bubbletea.KeyCtrlC {
			return engine.MsgCtrlC{}, true
		}
	}
	return nil, false
}

func toTeaCmd(effects []engine.Effect, a *TeaModelAdapter) bubbletea.Cmd {
	var cmds []bubbletea.Cmd
	for _, e := range effects {
		switch eff := e.(type) {
		case engine.PrintPayload:
			cmds = append(cmds, interpretPrint(eff))
		case engine.QuitPayload:
			cmds = append(cmds, bubbletea.Quit)
		default:
			if cmd := Interpret(e); cmd != nil {
				cmds = append(cmds, cmd)
			} else {
				cmds = append(cmds, a.Spinner.Tick)
			}
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	if len(cmds) == 1 {
		return cmds[0]
	}
	return bubbletea.Sequence(cmds...)
}

func interpretPrint(eff engine.PrintPayload) bubbletea.Cmd {
	if eff.Raw {
		return bubbletea.Sequence(
			bubbletea.Printf("%s", eff.Content),
			func() bubbletea.Msg { return msgPrintFinished{} },
		)
	}
	return bubbletea.Sequence(
		bubbletea.Println(eff.Content),
		func() bubbletea.Msg { return msgPrintFinished{} },
	)
}
