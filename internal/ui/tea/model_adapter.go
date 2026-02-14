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
	Sink    FrameSink
}

// NewTeaModelAdapter creates an adapter with the given state and deps factory.
// The factory receives the spinner so ViewTool can render with the current spinner frame.
// sink must be non-nil; use NoopSink{} if observability is not needed.
func NewTeaModelAdapter(state *engine.State, factory DepsFactory, sink FrameSink) *TeaModelAdapter {
	if sink == nil {
		panic("tea: FrameSink must be non-nil for 100% frame observability")
	}
	s := spinner.New()
	s.Spinner = spinner.Dot
	deps := factory(&s)
	return &TeaModelAdapter{
		State:   state,
		Deps:    deps,
		Spinner: s,
		Sink:    sink,
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

// View delegates to engine.Render and emits ViewRendered to the sink.
func (a *TeaModelAdapter) View() string {
	deps := a.Deps
	deps.Spinner = &spinnerProvider{m: &a.Spinner}
	view := engine.Render(a.State, deps)
	s := a.State
	a.Sink.OnFrameEvent(FrameEvent{
		Type:     FrameEventViewRendered,
		View:     view,
		Snapshot: &RenderSnapshot{
			TotalFlushedLines: s.TotalFlushedLines,
			MaxAbsoluteHeight: s.MaxAbsoluteHeight,
			PrintQueueLen:     len(s.PrintQueue),
			IsPrinting:        s.IsPrinting,
		},
	})
	return view
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
			a.Sink.OnFrameEvent(FrameEvent{
				Type:    FrameEventHistoryFlushed,
				Content: eff.Content,
				Raw:     eff.Raw,
			})
			cmds = append(cmds, a.Sink.PrintCmd(eff.Content, eff.Raw))
		case engine.QuitPayload:
			a.Sink.OnFrameEvent(FrameEvent{Type: FrameEventQuitRequested})
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
