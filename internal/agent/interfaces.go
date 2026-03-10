package agent

import (
	"github.com/Cyclone1070/iav/internal/domain"
)

// toolRegistry provides tool storage and lookup.
type toolRegistry interface {
	Declarations() []domain.Declaration
	Get(name string) (domain.Tool, bool)
}

// eventSender defines the interface for sending UI updates from the agent.
type eventSender interface {
	SendUIUpdate(domain.UIUpdate)
}
