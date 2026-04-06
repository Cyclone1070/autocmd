package demoutil

import (
	"sync"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/eventbus"
)

// ToolTracker is a tiny demo helper that ensures every ToolStartEvent
// is eventually paired with a ToolEndEvent, even when the demo is cancelled.
type ToolTracker struct {
	bus *eventbus.EventBus

	mu   sync.Mutex
	open map[string]trackedTool
}

type trackedTool struct {
	callID   string
	display  domain.ToolDisplay
}

func NewToolTracker(bus *eventbus.EventBus) *ToolTracker {
	return &ToolTracker{
		bus:  bus,
		open: make(map[string]trackedTool),
	}
}

func (t *ToolTracker) Start(callID string, display domain.ToolDisplay) {
	if t == nil || t.bus == nil {
		return
	}

	t.mu.Lock()
	t.open[callID] = trackedTool{
		callID:  callID,
		display: display,
	}
	t.mu.Unlock()

	t.bus.SendUIUpdate(domain.ToolStartEvent{
		CallID:   callID,
		Display:  display,
	})
}

func (t *ToolTracker) End(callID string, display domain.ToolDisplay) {
	if t == nil || t.bus == nil {
		return
	}

	t.mu.Lock()
	delete(t.open, callID)
	t.mu.Unlock()

	t.bus.SendUIUpdate(domain.ToolEndEvent{
		CallID:  callID,
		Display: display,
	})
}

func (t *ToolTracker) FlushOpenCancelled() {
	if t == nil || t.bus == nil {
		return
	}

	t.mu.Lock()
	open := make([]trackedTool, 0, len(t.open))
	for _, v := range t.open {
		open = append(open, v)
	}
	t.open = make(map[string]trackedTool)
	t.mu.Unlock()

	for _, tool := range open {
		t.bus.SendUIUpdate(domain.ToolEndEvent{
			CallID:  tool.callID,
			Display: withCancelledError(tool.display),
		})
	}
}

func withCancelledError(d domain.ToolDisplay) domain.ToolDisplay {
	if _, ok := d.(domain.QuestionDisplay); ok {
		return domain.StringDisplay{TypeField: "string", Content: "Question dismissed", Error: domain.ToolErrorCancelled}
	}
	return d.WithError(domain.ToolErrorCancelled)
}

