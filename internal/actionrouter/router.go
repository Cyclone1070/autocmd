// Package actionrouter handles routing of user actions to their respective handlers.
package actionrouter

import (
	"context"

	"github.com/Cyclone1070/iav/internal/domain"
)

// Router handles delivery of structured tool actions (like QuestionAnswerAction)
// from the UI bus to the blocked tool executor.
type Router struct {
	in   chan domain.Action
	out  chan domain.Action
	done chan struct{}
}

const channelBufferSize = 100

// New creates a new action router.
func New() *Router {
	r := &Router{
	in:   make(chan domain.Action, channelBufferSize),
		out:  make(chan domain.Action, channelBufferSize),
		done: make(chan struct{}),
	}
	go r.runBroker()
	return r
}

func (r *Router) runBroker() {
	var queue []domain.Action
	for {
		var out chan domain.Action
		var next domain.Action
		if len(queue) > 0 {
			out = r.out
			next = queue[0]
		}
		select {
		case <-r.done:
			return
		case act := <-r.in:
			queue = append(queue, act)
		case out <- next:
			queue = queue[1:]
		}
	}
}

// Close stops the background broker queue.
func (r *Router) Close() {
	close(r.done)
}

// Wait blocks until an action with the matching CallID is delivered, or the context is cancelled.
func (r *Router) Wait(ctx context.Context, callID string) (domain.Action, error) {
	for {
		select {
		case act := <-r.out:
			c, ok := act.(domain.CallIDer)
			if ok && c.GetCallID() == callID {
				return act, nil
			}
			// Mismatch found, discarding stale action
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// Deliver routes an action to its waiting consumer based on CallIDer interface.
func (r *Router) Deliver(act domain.Action) {
	_, ok := act.(domain.CallIDer)
	if !ok {
		return
	}

	select {
	case r.in <- act:
	case <-r.done:
	}
}
