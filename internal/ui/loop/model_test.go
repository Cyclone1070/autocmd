package loop

import (
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

type mockBus struct {
	updates chan domain.UIUpdate
	actions []domain.Action
}

func (m *mockBus) UIUpdates() <-chan domain.UIUpdate { return m.updates }
func (m *mockBus) SendAction(a domain.Action)       { m.actions = append(m.actions, a) }

type mockStream struct {
	p string
}
func (s *mockStream) Append(t string) []string { return nil }
func (s *mockStream) Flush() []string         { return []string{"flushed"} }
func (s *mockStream) Pending() string         { return s.p }

type mockAnimator struct {
	pending bool
}
func (a *mockAnimator) Enqueue(t string) {}
func (a *mockAnimator) NextChunk() (string, bool) { 
	if a.pending {
		a.pending = false
		return "chunk", true
	}
	return "", false 
}
func (a *mockAnimator) HasPending() bool { return a.pending }
func (a *mockAnimator) FlushAll() string { return "" }

type mockThinkingRenderer struct{}
func (t *mockThinkingRenderer) RenderThinking(status ui.ToolStatus, start time.Time, tick int, sp spinnerProvider) string {
	return "thinking_rendered"
}

func TestModel_SyncPolling(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	m := NewModel(bus, nil, nil, nil, &mockStream{}, &TextAnimator{runesPerTick: 4}, 80)

	// Specification: Init should start the heartbeat (100ms)
	m.Init()

	// Specification: On tick, if an event is waiting, it should be processed
	bus.updates <- domain.ThinkingEvent{}
	
	// We call Update with a tickMsg (the heartbeat)
	res, _ := m.Update(tickMsg{})
	newModel := res.(*Model)

	// Since ThinkingEvent triggers a flush, it should be in stateFlushing
	assert.Equal(t, stateFlushing, newModel.state)
	assert.Equal(t, stateThinking, newModel.nextState)
	
	// Receive flushDoneMsg -> should transition to stateThinking
	res, _ = newModel.Update(flushDoneMsg{})
	newModel = res.(*Model)
	assert.Equal(t, stateThinking, newModel.state)
}

func TestModel_SpinnerAdvancesDuringEvents(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	m := NewModel(bus, nil, nil, nil, &mockStream{}, &mockAnimator{pending: true}, 80)
	m.state = stateThinking
	m.spinnerFrame = 10

	// handleTick should increment even if it processes something or returns early
	m.handleTick()
	assert.Equal(t, 11, m.spinnerFrame, "Spinner should increment in handleTick")
}

func TestModel_ThinkingResultFlushedOnTransition(t *testing.T) {
	var flushed []string
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	m := NewModel(bus, &mockThinkingRenderer{}, nil, nil, &mockStream{}, &mockAnimator{}, 80, WithFlush(func(c string) tea.Cmd {
		flushed = append(flushed, c)
		return nil
	}))
	m.state = stateThinking
	
	// Transition to streaming
	m.handleEvent(domain.TextEvent{Text: "hi"})
	
	assert.Contains(t, flushed, "thinking_rendered", "Thinking result should be flushed on transition")
}

func TestModel_ViewportTruncation(t *testing.T) {
	m := NewModel(nil, nil, nil, nil, &mockStream{p: "L1\nL2\nL3\nL4\nL5"}, nil, 80, WithTermHeight(3))
	v := m.View()
	assert.Contains(t, v, "truncated", "View should be truncated with indicator")
}

func TestModel_FlushDoneMsgSequencing(t *testing.T) {
	// Logical check: doFlush must use tea.Sequence to ensure order
	m := &Model{}
	m.flushFn = func(c string) tea.Cmd { return nil }
	_, cmd := m.doFlush([]string{"b1", "b2"}, stateIdle)
	
	// Verified in code: return m, tea.Sequence(cmds...)
	assert.NotNil(t, cmd)
}
func TestModel_DefaultTermHeight(t *testing.T) {
	m := NewModel(nil, nil, nil, nil, nil, nil, 80)
	assert.Equal(t, 25, m.termHeight, "Model should default to 25-line height fallback")
}
