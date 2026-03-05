package agent

import (
	"sync"

	"github.com/Cyclone1070/iav/internal/domain"
)

// EventBroker buffers events from producers (agent loop) and delivers them
// asynchronously to a consumer channel (UI). This ensures the producer
// loop is never blocked by UI rendering speed.
type EventBroker struct {
	in     chan domain.Event
	out    chan<- domain.Event
	closed chan struct{}
	wg     sync.WaitGroup
}

// NewEventBroker creates and starts a new EventBroker.
func NewEventBroker(out chan<- domain.Event) *EventBroker {
	b := &EventBroker{
		in:     make(chan domain.Event),
		out:    out,
		closed: make(chan struct{}),
	}
	b.wg.Add(1)
	go b.run()
	return b
}

// Send queues an event. It never blocks.
func (b *EventBroker) Send(ev domain.Event) {
	select {
	case <-b.closed:
		return
	case b.in <- ev:
	}
}

// Close signals that no more events will be sent.
// It blocks until all already-buffered events have been delivered to the output channel.
// It DOES NOT close the output channel.
func (b *EventBroker) Close() {
	select {
	case <-b.closed:
		return
	default:
		close(b.in)
		b.wg.Wait()
		close(b.closed)
	}
}

func (b *EventBroker) run() {
	defer b.wg.Done()

	var queue []domain.Event
	in := b.in

	for {
		var out chan<- domain.Event
		var next domain.Event
		if len(queue) > 0 {
			out = b.out
			next = queue[0]
		}

		select {
		case ev, ok := <-in:
			if !ok {
				// Input closed. Drain remaining queue then exit.
				// We use a non-blocking send for the drain to prevent deadlocks
				// if the UI has already stopped reading (e.g. on Ctrl+C).
				for _, qe := range queue {
					select {
					case b.out <- qe:
					default:
						return
					}
				}
				return
			}
			queue = append(queue, ev)
		case out <- next:
			queue = queue[1:]
		}
	}
}
