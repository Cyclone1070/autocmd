package agent

import (
	"github.com/Cyclone1070/iav/internal/domain"
)

// toolRegistry provides tool storage and lookup.
type toolRegistry interface {
	Declarations() []domain.Declaration
	Get(name string) (domain.Tool, bool)
}

// eventSender allows sending domain events.
// This is the consumer-defined interface injected into Loop.
type eventSender interface {
	Send(domain.Event)
}
