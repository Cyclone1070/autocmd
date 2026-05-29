package agent

import (
	"context"

	"github.com/Cyclone1070/autocmd/internal/domain"
	einotool "github.com/cloudwego/eino/components/tool"
)

// toolRegistry provides tool storage and lookup.
type toolRegistry interface {
	Tools() []einotool.BaseTool
	Get(name string) (einotool.BaseTool, bool)
}

// actionWaiter defines the interface for waiting on tool-specific user actions.
type actionWaiter interface {
	Wait(ctx context.Context, callID string) (domain.Action, error)
}

// eventSender defines the interface for sending UI updates from the agent.
type eventSender interface {
	SendUIUpdate(domain.UIUpdate)
}

type taskNotifier interface {
	// Drain returns all completed background tasks since the last call.
	Drain() []domain.TaskResult
	// HasRunning returns true if there are still active background tasks.
	HasRunning() bool
}
