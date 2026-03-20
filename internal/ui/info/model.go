package info

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}

	switch msg := msg.(type) {
	case domain.InfoEvent:
		// Print info immediately and keep polling
		return m, tea.Sequence(
			tea.Printf("%s", m.renderInfo(&msg)),
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
			// Styled error message via tea.Printf in a sequence with tea.Quit
			return tea.Sequence(
				tea.Printf("\n %s\n", m.theme.Error("Error: bus closed unexpectedly")),
				tea.Quit,
			)()
		}
		return ev
	}
}

// renderInfo handles formatting of info data to a string.
func (m *Model) renderInfo(data *domain.InfoEvent) string {
	var sb strings.Builder
	labelStyle := lipgloss.NewStyle().Bold(true)

	formatLine := func(label, value string) string {
		padding := 22
		rawLabel := fmt.Sprintf("%-*s", padding, label)
		return fmt.Sprintf("%s %s\n", labelStyle.Render(rawLabel), value)
	}

	// Model Section
	if data.Model != "" {
		sb.WriteString(formatLine("Model:", data.Model))
	}

	// Session Section
	sb.WriteString(formatLine("Current Session:", data.SessionDisplay))

	// Usage Section (only if model and context window are present)
	if data.Model != "" && data.ContextWindow > 0 {
		if data.SessionTokens > 0 {
			usagePct := float64(data.SessionTokens) / float64(data.ContextWindow) * 100
			usage := fmt.Sprintf("%d tokens (%.1f%% of %d context)", data.SessionTokens, usagePct, data.ContextWindow)
			sb.WriteString(formatLine("Session Usage:", usage))
		} else {
			sb.WriteString(formatLine("Context Window:", fmt.Sprintf("%d tokens", data.ContextWindow)))
		}
	}

	// Authorized Providers Section
	if len(data.Authorized) > 0 {
		sb.WriteString(formatLine("Authorized Providers:", strings.Join(data.Authorized, ", ")))
	}

	return sb.String()
}
