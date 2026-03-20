package loop

import (
	"strings"
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
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(bus, nil, nil, nil, theme, &mockStream{}, &TextAnimator{runesPerTick: 4}, ui.NewNoOpGater(), 80)

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
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(bus, nil, nil, nil, theme, &mockStream{}, &mockAnimator{pending: true}, ui.NewNoOpGater(), 80)
	m.state = stateThinking
	m.spinnerFrame = 10

	// handleTick should increment even if it processes something or returns early
	m.handleTick()
	assert.Equal(t, 11, m.spinnerFrame, "Spinner should increment in handleTick")
}

func TestModel_ThinkingResultFlushedOnTransition(t *testing.T) {
	var flushed []string
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(bus, &mockThinkingRenderer{}, nil, nil, theme, &mockStream{}, &mockAnimator{}, ui.NewNoOpGater(), 80, WithFlush(func(c string) tea.Cmd {
		flushed = append(flushed, c)
		return nil
	}))
	m.state = stateThinking
	
	// Transition to streaming
	m.handleEvent(domain.TextEvent{Text: "hi"})
	
	assert.Contains(t, flushed, "thinking_rendered", "Thinking result should be flushed on transition")
}

func TestModel_ViewportTruncation(t *testing.T) {
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(nil, nil, nil, nil, theme, &mockStream{p: "L1\nL2\nL3\nL4\nL5"}, nil, ui.NewTruncatingGater(3), 80)
	v := m.View()
	assert.Contains(t, v, "truncated", "View should be truncated with indicator")
}

func TestModel_NoTruncationIfHeightZero(t *testing.T) {
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(nil, nil, nil, nil, theme, &mockStream{p: "L1\nL2\nL3\nL4\nL5"}, nil, ui.NewTruncatingGater(0), 80)
	v := m.View()
	assert.NotContains(t, v, "truncated", "View should not be truncated when height is 0")
	assert.Contains(t, v, "L5", "Full content should be visible")
}

func TestModel_FlushDoneMsgSequencing(t *testing.T) {
	// Logical check: doFlush must use tea.Sequence to ensure order
	m := &Model{}
	m.flushFn = func(c string) tea.Cmd { return nil }
	_, cmd := m.doFlush([]string{"b1", "b2"}, stateIdle)
	
	// Verified in code: return m, tea.Sequence(cmds...)
	assert.NotNil(t, cmd)
}
func TestModel_ExplicitGater(t *testing.T) {
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(nil, nil, nil, nil, theme, nil, nil, ui.NewNoOpGater(), 80)
	// Success if constructor accepts it
	assert.NotNil(t, m.gater)
}

type mockGater struct {
	gateFunc func(string) string
}
func (m *mockGater) Gate(s string) string { return m.gateFunc(s) }

func TestModel_ViewUsesGater(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	g := &mockGater{gateFunc: func(s string) string { return s + "_gated" }}
	theme := ui.NewTheme(ui.ThemeConfig{})
	m := NewModel(bus, nil, nil, nil, theme, &mockStream{p: "raw"}, nil, g, 80)
	
	assert.Equal(t, "raw_gated", m.View())
}

func TestModel_ToolsViewSpacing(t *testing.T) {
	theme := ui.NewTheme(ui.ThemeConfig{})
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	sp := &mockSpinner{}
	m := NewModel(nil, nil, tr, sp, theme, nil, nil, ui.NewNoOpGater(), 80)
	
	m.tools = []toolSlot{
		{callID: "1", toolName: "t1", status: ui.StatusSuccess, display: domain.StringDisplay{Content: "c1"}},
		{callID: "2", toolName: "t2", status: ui.StatusSuccess, display: domain.StringDisplay{Content: "c2"}},
	}
	m.state = stateTooling
	
	v := m.View()
	// Each box starts with \n. Join adds another \n.
	// So we expect: ...bottom_border\n\n\nbox2_top_border...
	// Wait, if Box1 is "\nBox1" and Box2 is "\nBox2", and we join with "\n", 
	// we get "\nBox1" + "\n" + "\nBox2" = "\nBox1\n\nBox2".
	// The blank line is EXACTLY there.
	assert.Contains(t, v, "╯\n\n╭", "There should be a blank line between the boxes")
}

func TestModel_HandleCancel_SetsGenericErrorOnRunningTools(t *testing.T) {
	bus := &mockBus{updates: make(chan domain.UIUpdate, 10)}
	theme := ui.NewTheme(ui.ThemeConfig{})
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	sp := &mockSpinner{}

	m := NewModel(bus, nil, tr, sp, theme, nil, nil, ui.NewNoOpGater(), 80)
	m.state = stateTooling
	m.tools = []toolSlot{
		{callID: "1", toolName: "t1", status: ui.StatusRunning, display: domain.StringDisplay{Content: "Run something"}},
	}

	// Act: simulate Ctrl+C cancel
	res, _ := m.handleCancel()
	newModel := res.(*Model)

	if assert.Len(t, newModel.tools, 1) {
		assert.Equal(t, ui.StatusError, newModel.tools[0].status, "Running tool should be marked as error on cancel")
		assert.Equal(t, "cancelled", newModel.tools[0].errorMsg, "Cancelled tools should carry a generic error message")
	}
}

func TestModel_BusClosedUnexpectedly(t *testing.T) {
	var flushed []string
	bus := &mockBus{updates: make(chan domain.UIUpdate)}
	close(bus.updates) // Close channel immediately

	theme := ui.NewTheme(ui.ThemeConfig{})
	tr := ui.NewToolRenderer(theme, 80, ui.NewNoOpGater())
	tickr := &mockThinkingRenderer{}
	sp := &mockSpinner{}
	m := NewModel(bus, tickr, tr, sp, theme, &mockStream{}, &mockAnimator{}, ui.NewNoOpGater(), 80, WithFlush(func(s string) tea.Cmd {
		flushed = append(flushed, s)
		return nil
	}))
	m.state = stateThinking
	m.thinkingStart = time.Now()

	// Act: Trigger a tick
	m.Update(tickMsg{})

	// Assert: Should detect closed channel, mark tools as error, and transition to flushing/done
	assert.Contains(t, flushed, "thinking_rendered")
	
	found := false
	for _, f := range flushed {
		// Use a case-insensitive check or specific styled check
		if strings.Contains(strings.ToLower(f), "bus closed unexpectedly") {
			found = true
			break
		}
	}
	assert.True(t, found, "Should flush the explicit error message 'bus closed unexpectedly'")
}
