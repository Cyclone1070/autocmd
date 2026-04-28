// Package prompt provides the main interactive prompt and tool execution UI.
package prompt

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	tickHighDelay = 100 * time.Millisecond
)

// animationTick returns a Cmd that fires after the specified interval.
func animationTick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(_ time.Time) tea.Msg {
		return tickMsg{}
	})
}
