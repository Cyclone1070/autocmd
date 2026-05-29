// Package info provides UI components for displaying system information and settings.
package info

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const percentMultiplier = 100.0

type bus interface {
	UIUpdates() <-chan domain.UIUpdate
}

// Model is the bubbletea model for displaying system information.
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

// Init initializes the model and starts polling for updates.
func (m *Model) Init() tea.Cmd {
	return m.pollBus()
}

// Update handles UI updates and incoming information events.
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

// View renders the model (returns empty as info is printed via tea.Printf).
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
		return fmt.Sprintf("%s %s\n", labelStyle.Render(rawLabel), m.theme.Success(value))
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
			usagePct := float64(data.SessionTokens) / float64(data.ContextWindow) * percentMultiplier
			usage := fmt.Sprintf("%s tokens (%.1f%% of %s context)", ui.ShortNum(data.SessionTokens), usagePct, ui.ShortNum(data.ContextWindow))
			sb.WriteString(formatLine("Session Usage:", usage))
		} else {
			sb.WriteString(formatLine("Context Window:", fmt.Sprintf("%s tokens", ui.ShortNum(data.ContextWindow))))
		}
	}

	// Authorized Providers Section
	if len(data.Authorized) > 0 {
		sb.WriteString(formatLine("Authorized Providers:", strings.Join(data.Authorized, ", ")))
	}

	return sb.String()
}
