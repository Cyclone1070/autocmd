package agent

import (
	"context"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
)

// toolRegistry provides tool storage and lookup.
type toolRegistry interface {
	Definitions() []*schema.ToolInfo
	Get(name string) (domain.Tool, bool)
}

// actionWaiter defines the interface for waiting on tool-specific user actions.
type actionWaiter interface {
	Wait(ctx context.Context, callID string) (domain.Action, error)
}

// eventSender defines the interface for sending UI updates from the agent.
type eventSender interface {
	SendUIUpdate(domain.UIUpdate)
}
