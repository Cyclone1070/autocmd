package agent

import (
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
)

// toolRegistry provides tool storage and lookup.
type toolRegistry interface {
	Definitions() []*schema.ToolInfo
	Get(name string) (domain.Tool, bool)
}

// eventSender defines the interface for sending UI updates from the agent.
type eventSender interface {
	SendUIUpdate(domain.UIUpdate)
}
