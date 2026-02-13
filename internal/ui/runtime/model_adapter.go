package runtime

import (
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui/engine"
	tea "github.com/charmbracelet/bubbletea"
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
func (a *TeaModelAdapter) Init() tea.Cmd {
	return a.Spinner.Tick
}

// Update processes messages and returns updated model and command.
func (a *TeaModelAdapter) Update(teaMsg tea.Msg) (tea.Model, tea.Cmd) {
	// Wire spinner into deps for View
	deps := a.Deps
	deps.Spinner = &spinnerProvider{m: &a.Spinner}

	engineMsg, ok := toEngineMsg(teaMsg)
	if ok {
		_, effects := engine.Transition(a.State, engineMsg, deps)
		cmd := toTeaCmd(effects, a)
		return a, cmd
	}

	// Handle spinner tick
	if _, ok := teaMsg.(spinner.TickMsg); ok {
		var cmd tea.Cmd
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

// spinnerProvider adapts spinner.Model to engine.SpinnerViewProvider.
type spinnerProvider struct {
	m *spinner.Model
}

func (s *spinnerProvider) SpinnerView() string {
	return s.m.View()
}

// toEngineMsg converts tea.Msg to engine.Msg.
func toEngineMsg(teaMsg tea.Msg) (engine.Msg, bool) {
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
	case tea.KeyMsg:
		if ev.Type == tea.KeyCtrlC {
			return engine.MsgCtrlC{}, true
		}
	}
	return nil, false
}

// toTeaCmd converts engine effects to a single tea.Cmd.
func toTeaCmd(effects []engine.Effect, a *TeaModelAdapter) tea.Cmd {
	var cmds []tea.Cmd
	for _, e := range effects {
		switch eff := e.(type) {
		case engine.PrintPayload:
			cmds = append(cmds, interpretPrint(eff))
		case engine.QuitPayload:
			cmds = append(cmds, tea.Quit)
		default:
			// effectScheduleTick or other - Interpret returns nil for tick, so use spinner
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
	return tea.Sequence(cmds...)
}

func interpretPrint(eff engine.PrintPayload) tea.Cmd {
	if eff.Raw {
		return tea.Sequence(
			tea.Printf("%s", eff.Content),
			func() tea.Msg { return msgPrintFinished{} },
		)
	}
	return tea.Sequence(
		tea.Println(eff.Content),
		func() tea.Msg { return msgPrintFinished{} },
	)
}
