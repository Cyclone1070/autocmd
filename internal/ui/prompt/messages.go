package prompt

import "github.com/Cyclone1070/iav/internal/domain"

// tickMsg signals an animation tick for text streaming.
type tickMsg struct{}

// flushDoneMsg signals that a side-effect (print) has finished.
type flushDoneMsg struct{}

// busEventMsg wraps a UI update delivered by pollBus.
type busEventMsg struct {
	event domain.UIUpdate
}

// busClosedMsg signals the bus channel closed unexpectedly.
type busClosedMsg struct{}

// animatorDrainedMsg signals the streaming animator has no pending runes after a tick drain.
type animatorDrainedMsg struct{}
