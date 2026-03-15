package loop

import "github.com/Cyclone1070/iav/internal/domain"

// eventMsg carries a UIUpdate from the bus into Update().
type eventMsg struct {
	update domain.UIUpdate
}

// tickMsg signals an animation tick for text streaming.
type tickMsg struct{}

// channelClosedMsg signals the bus channel closed unexpectedly (before DoneEvent).
type channelClosedMsg struct{}
