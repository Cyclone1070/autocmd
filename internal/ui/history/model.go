package history

import (
	"strings"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model is the bubbletea model for the history viewer.
type Model struct {
	messages []domain.Message
	cfg      config.UIConfig
	theme    *ui.Theme
	width    int
	height   int
	renderer ui.Renderer
	viewport viewport.Model

	// Cache for lazy rendering
	renderedMessages map[int]string
	topIdx           int
	bottomIdx        int
	reachedTop       bool
	renderedBlock    string
	isDark           bool
}

// Option is a functional option for configuring the Model.
type Option func(*Model)

// WithRenderer sets the renderer for the model.
func WithRenderer(r ui.Renderer) Option {
	return func(m *Model) {
		m.renderer = r
	}
}

// NewModel creates a new history model.
func NewModel(messages []domain.Message, cfg config.UIConfig, width, height int, opts ...Option) *Model {
	m := &Model{
		messages:         messages,
		cfg:              cfg,
		theme:            ui.NewTheme(cfg),
		height:           height,
		renderedMessages: make(map[int]string),
		topIdx:           0,
		bottomIdx:        0,
		reachedTop:       false,
		isDark:           lipgloss.HasDarkBackground(),
	}
	m.width = m.calculateWidth(width)

	for _, opt := range opts {
		opt(m)
	}

	if m.renderer == nil {
		m.renderer, _ = ui.NewGlamourRenderer(m.width, m.isDark)
	}

	m.viewport = viewport.New(m.width, height)
	m.initializeContent()

	return m
}

func (m *Model) initializeContent() {
	if len(m.messages) == 0 {
		m.reachedTop = true
		return
	}

	m.bottomIdx = len(m.messages) - 1
	m.topIdx = m.bottomIdx

	var renderedParts []string
	currentHeight := 0
	// Render enough to fill the viewport plus one screen height of buffer.
	limit := m.height * 2

	for m.topIdx >= 0 {
		rendered := m.renderMessage(m.topIdx)
		renderedParts = append([]string{rendered}, renderedParts...)
		currentHeight += lipgloss.Height(rendered)
		m.topIdx--

		if currentHeight >= limit {
			break
		}
	}
	if m.topIdx < 0 {
		m.reachedTop = true
		m.topIdx = 0
	}

	m.renderedBlock = strings.Join(renderedParts, "")
	m.viewport.SetContent(m.renderedBlock)
	m.viewport.GotoBottom()
}

func (m *Model) renderMessage(idx int) string {
	if r, ok := m.renderedMessages[idx]; ok {
		return r
	}
	rendered := RenderMessage(m.messages, idx, m.renderer, m.theme, m.width, idx > 0)
	m.renderedMessages[idx] = rendered
	return rendered
}

func (m *Model) refreshViewport() {
	// If we are getting close to the top of the rendered block, prepend more
	// Use one screen height as the safety margin.
	for !m.reachedTop && m.viewport.YOffset < m.height {
		rendered := m.renderMessage(m.topIdx)
		h := lipgloss.Height(rendered)

		m.renderedBlock = rendered + m.renderedBlock
		m.viewport.SetContent(m.renderedBlock)

		// Shift YOffset down so the user stays at the same visual location relative to bottom
		m.viewport.YOffset += h
		m.topIdx--
		if m.topIdx < 0 {
			m.reachedTop = true
			m.topIdx = 0
		}
	}
}

func (m *Model) calculateWidth(termWidth int) int {
	w := m.cfg.ChatWindowWidth
	if termWidth > 0 && termWidth < w {
		w = termWidth
	}
	return w
}

func (m *Model) Init() tea.Cmd {
	return nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = m.calculateWidth(msg.Width)
		m.renderer, _ = ui.NewGlamourRenderer(m.width, m.isDark)
		m.viewport.Width = m.width
		m.viewport.Height = m.height
		// Reset everything on resize.
		m.renderedMessages = make(map[int]string)
		m.initializeContent()
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	m.refreshViewport()

	return m, tea.Batch(cmds...)
}

func (m *Model) View() string {
	return m.viewport.View()
}
