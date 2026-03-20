package info

import (
	"fmt"
	"os"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

type bus interface {
	UIUpdates() <-chan domain.UIUpdate
}

type Model struct {
	bus bus
}

func NewModel(b bus) *Model {
	return &Model{
		bus: b,
	}
}

func (m *Model) Init() tea.Cmd {
	return m.pollBus()
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case *domain.InfoEvent:
		// Print immediately when data is received
		return m, tea.Sequence(
			tea.Printf("%s", renderInfo(msg)),
			m.pollBus(),
		)
	case domain.DoneEvent:
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) View() string {
	return ""
}

func (m *Model) pollBus() tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-m.bus.UIUpdates()
		if !ok {
			fmt.Fprintln(os.Stderr, "error: bus closed prematurely")
			return domain.DoneEvent{}
		}
		return ev
	}
}

// renderInfo handles formatting of info data to a string.
func renderInfo(data *domain.InfoEvent) string {
	var sb strings.Builder

	// Model Section
	if data.Model != "" {
		sb.WriteString(fmt.Sprintf("\033[1m%-22s\033[0m %s\n", "Model:", data.Model))
	}

	// Session Section
	sb.WriteString(fmt.Sprintf("\033[1m%-22s\033[0m %s\n", "Current Session:", data.SessionDisplay))

	// Usage Section (only if model and tokens/window are present)
	if data.Model != "" && data.ContextWindow > 0 {
		if data.SessionTokens > 0 {
			usagePct := float64(data.SessionTokens) / float64(data.ContextWindow) * 100
			sb.WriteString(fmt.Sprintf("\033[1m%-22s\033[0m %d tokens (%.1f%% of %d context)\n", "Session Usage:", data.SessionTokens, usagePct, data.ContextWindow))
		} else {
			sb.WriteString(fmt.Sprintf("\033[1m%-22s\033[0m %d tokens\n", "Context Window:", data.ContextWindow))
		}
	}

	// Authorized Providers Section
	if len(data.Authorized) > 0 {
		sb.WriteString(fmt.Sprintf("\033[1m%-22s\033[0m %s\n", "Authorized Providers:", strings.Join(data.Authorized, ", ")))
	}

	return sb.String()
}
