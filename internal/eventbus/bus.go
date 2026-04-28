package eventbus

import (
	"sync"

	"github.com/Cyclone1070/iav/internal/domain"
)

const (
	busBufferSize = 100
	numWorkers    = 2
)

// EventBus facilitates bi-directional communication between the UI and Workflow.
// It buffers updates to the UI and actions to the workflow.
type EventBus struct {
	uiIn  chan domain.UIUpdate
	uiOut chan domain.UIUpdate

	workflowIn  chan domain.Action
	workflowOut chan domain.Action

	mu       sync.RWMutex
	isClosed bool
	done     chan struct{}
	wg       sync.WaitGroup
}

// New creates and starts a new EventBus.
func New() *EventBus {
	b := &EventBus{
		uiIn:        make(chan domain.UIUpdate, busBufferSize),
		uiOut:       make(chan domain.UIUpdate, busBufferSize),
		workflowIn:  make(chan domain.Action, busBufferSize),
		workflowOut: make(chan domain.Action, busBufferSize),
		done:        make(chan struct{}),
	}
	b.wg.Add(numWorkers)
	go b.runUIUpdates()
	go b.runWorkflowActions()
	return b
}

// UIUpdates returns the channel for receiving UI updates.
func (b *EventBus) UIUpdates() <-chan domain.UIUpdate {
	return b.uiOut
}

// WorkflowActions returns the channel for receiving workflow actions.
func (b *EventBus) WorkflowActions() <-chan domain.Action {
	return b.workflowOut
}

// SendUIUpdate queues an update for the UI. It never blocks.
func (b *EventBus) SendUIUpdate(upd domain.UIUpdate) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.isClosed {
		return
	}
	b.uiIn <- upd
}

// SendAction queues an action for the workflow. It never blocks.
func (b *EventBus) SendAction(act domain.Action) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.isClosed {
		return
	}
	b.workflowIn <- act
}

// Close signals that no more updates or actions will be sent.
// It blocks until all already-buffered updates have been delivered.
func (b *EventBus) Close() {
	b.mu.Lock()
	if b.isClosed {
		b.mu.Unlock()
		return
	}
	b.isClosed = true
	close(b.uiIn)
	close(b.workflowIn)
	b.mu.Unlock()

	b.wg.Wait()
	close(b.done)
}

func (b *EventBus) runUIUpdates() {
	defer b.wg.Done()

	var queue []domain.UIUpdate
	in := b.uiIn

	for {
		var out chan domain.UIUpdate
		var next domain.UIUpdate
		if len(queue) > 0 {
			out = b.uiOut
			next = queue[0]
		}

		select {
		case upd, ok := <-in:
			if !ok {
				close(b.uiOut)
				return
			}
			queue = append(queue, upd)

		case out <- next:
			queue = queue[1:]
		}
	}
}

func (b *EventBus) runWorkflowActions() {
	defer b.wg.Done()

	var queue []domain.Action
	in := b.workflowIn

	for {
		var out chan domain.Action
		var next domain.Action
		if len(queue) > 0 {
			out = b.workflowOut
			next = queue[0]
		}

		select {
		case act, ok := <-in:
			if !ok {
				close(b.workflowOut)
				return
			}
			queue = append(queue, act)

		case out <- next:
			queue = queue[1:]
		}
	}
}
