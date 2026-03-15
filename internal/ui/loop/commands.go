package loop

import (
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

// waitForEvent blocks on the bus channel and returns the event as a tea.Msg.
func waitForEvent(ch <-chan domain.UIUpdate) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return channelClosedMsg{}
		}
		return eventMsg{update: ev}
	}
}

const animationTickInterval = 100 * time.Millisecond

// animationTick returns a Cmd that fires after the animation interval.
func animationTick() tea.Cmd {
	return tea.Tick(animationTickInterval, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

// batchWaitAndFlush batches a flush Cmd with waitForEvent. If flushCmd is nil, only waitForEvent is returned.
func batchWaitAndFlush(flushCmd tea.Cmd, ch <-chan domain.UIUpdate) tea.Cmd {
	if flushCmd == nil {
		return waitForEvent(ch)
	}
	return tea.Batch(flushCmd, waitForEvent(ch))
}
