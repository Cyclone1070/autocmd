package prompt

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	tickLowDelay  = 16 * time.Millisecond
	tickHighDelay = 100 * time.Millisecond
)

// animationTick returns a Cmd that fires after the specified interval.
func animationTick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

// signalAnimatorDrained enqueues animatorDrainedMsg so the poll loop can resume if parked.
func signalAnimatorDrained() tea.Cmd {
	return func() tea.Msg {
		return animatorDrainedMsg{}
	}
}
