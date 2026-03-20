package info

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

type bus interface {
	UIUpdates() <-chan domain.UIUpdate
}

type Model struct {
	bus   bus
	theme *ui.Theme
}

// NewModel creates a new reactive InfoModel.
func NewModel(b bus, th *ui.Theme) *Model {
	return &Model{
		bus:   b,
		theme: th,
	}
}

func (m *Model) Init() tea.Cmd {
	return m.pollBus()
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case domain.InfoEvent:
		// Print the info immediately and keep polling
		return m, tea.Sequence(
			tea.Printf("%s", renderInfo(&msg)),
			m.pollBus(),
		)

	case domain.DoneEvent:
		// Termination signal
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
			// Styled error message
			msg := "\n " + m.theme.Error("Error: bus closed unexpectedly") + "\n"
			return tea.Printf("%s", msg)()
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
