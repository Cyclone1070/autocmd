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

// WithIsDark sets the dark mode flag for the model.
func WithIsDark(isDark bool) Option {
	return func(m *Model) {
		m.isDark = isDark
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
		isDark:           false, // Default, will be overridden by options or polled on-demand
	}
	m.width = m.calculateWidth(width)

	for _, opt := range opts {
		opt(m)
	}

	// If isDark wasn't provided, we poll it. But ideally it's passed from cmd/
	if !m.isDark && m.renderer == nil {
		m.isDark = lipgloss.HasDarkBackground()
	}

	if m.renderer == nil {
		m.renderer = ui.NewGlamourRenderer(m.width, m.isDark)
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

	m.reachedTop = false
	m.bottomIdx = len(m.messages) - 1
	m.topIdx = m.bottomIdx

	var renderedParts []string
	currentHeight := 0
	// Render enough to fill the viewport plus one screen height of buffer.
	limit := m.height * 2

	for m.topIdx >= 0 {
		rendered := m.renderMessage(m.topIdx)
		renderedParts = append([]string{rendered}, renderedParts...)

		// Use exact height of joined parts to avoid the newline fusion bug
		m.renderedBlock = strings.Join(renderedParts, "")
		currentHeight = lipgloss.Height(m.renderedBlock)
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

		// Record the previous line count before we add the new string
		oldTotal := m.viewport.TotalLineCount()

		m.renderedBlock = rendered + m.renderedBlock
		m.viewport.SetContent(m.renderedBlock)

		// Calculate exactly how many virtual lines were added.
		// We do this instead of lipgloss.Height() because joining two strings
		// that both end in newlines results in 1 fewer line than the sum of their individual heights.
		linesAdded := m.viewport.TotalLineCount() - oldTotal

		// Shift YOffset down by exactly the number of lines introduced
		m.viewport.YOffset += linesAdded
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

	// Update sub-models first so they have correct dimensions before our logic runs
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		newWidth := m.calculateWidth(msg.Width)
		if newWidth != m.width {
			m.width = newWidth
			m.renderer = ui.NewGlamourRenderer(m.width, m.isDark)
			// Reset rendered cache only on width change as it affects wrapping.
			m.renderedMessages = make(map[int]string)
		}
		m.height = msg.Height
		m.viewport.Width = m.width
		m.viewport.Height = m.height
		// Always re-initialize content to fill the new viewport and anchor to bottom
		m.initializeContent()
	}

	m.refreshViewport()

	return m, tea.Batch(cmds...)
}

func (m *Model) View() string {
	return m.viewport.View()
}
