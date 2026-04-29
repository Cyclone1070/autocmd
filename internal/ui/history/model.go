package history

import (
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cloudwego/eino/schema"
	"golang.org/x/term"
)

const preloadFactor = 2

type bus interface {
	UIUpdates() <-chan domain.UIUpdate
	SendAction(domain.Action)
}

// Model is the bubbletea model for the history viewer.
type Model struct {
	bus              bus
	renderer         ui.Renderer
	theme            *ui.Theme
	renderedMessages map[int]string
	builder          *Builder
	displays         domain.ToolDisplays
	renderedBlock    string
	messages         []*schema.Message
	items            []renderItem
	viewport         viewport.Model
	height           int
	bashOutputHeight int
	width            int
	topIdx           int
	bottomIdx        int
	chatWindowWidth  int
	loaded           bool
	reachedTop       bool
	isDark           bool
}

// Option is a functional option for configuring the Model.
type Option func(*Model)

// WithRenderer sets the renderer for the Model.
func WithRenderer(r ui.Renderer) Option {
	return func(m *Model) {
		m.renderer = r
	}
}

// WithIsDark sets the dark mode flag for the Model.
func WithIsDark(isDark bool) Option {
	return func(m *Model) {
		m.isDark = isDark
	}
}

// NewModel creates a new history Model.
func NewModel(b bus, theme *ui.Theme, chatWindowWidth int, bashOutputHeight int, width, height int, opts ...Option) *Model {
	m := &Model{
		bus:              b,
		theme:            theme,
		chatWindowWidth:  chatWindowWidth,
		bashOutputHeight: bashOutputHeight,
		height:           height,
		renderedMessages: make(map[int]string),
		topIdx:           0,
		bottomIdx:        0,
		reachedTop:       false,
		isDark:           false,
	}
	m.width = m.calculateWidth(width)

	for _, opt := range opts {
		opt(m)
	}

	if !m.isDark && m.renderer == nil {
		if fd, err := toIntSafe(os.Stdout.Fd()); err == nil && term.IsTerminal(fd) {
			m.isDark = lipgloss.HasDarkBackground()
		}
	}

	if m.renderer == nil {
		renderWidth := m.width - gutterWidth
		m.renderer = ui.NewGlamourRenderer(renderWidth, m.isDark)
	}

	m.syncBuilder()

	m.viewport = viewport.New(m.width, height)
	return m
}

func (m *Model) syncBuilder() {
	m.builder = NewBuilder(m.renderer, m.theme, m.width, m.bashOutputHeight)
}

func (m *Model) initializeContent() {
	m.items = buildRenderItems(m.messages)
	if len(m.items) == 0 {
		m.reachedTop = true
		return
	}

	m.reachedTop = false
	m.bottomIdx = len(m.items) - 1
	m.topIdx = m.bottomIdx

	var renderedParts []string
	var currentHeight int
	limit := m.height * preloadFactor

	for m.topIdx >= 0 {
		rendered := m.renderMessage(m.topIdx)
		renderedParts = append([]string{rendered}, renderedParts...)

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
	it := m.items[idx]
	if len(it.assistantIndices) > 0 {
		rendered := m.builder.renderCoalescedAssistant(m.messages, it.assistantIndices, m.displays, it.assistantCancelled)
		m.renderedMessages[idx] = rendered
		return rendered
	}
	rendered := m.builder.RenderMessage(m.messages, it.idx, m.displays, idx > 0)
	m.renderedMessages[idx] = rendered
	return rendered
}

func (m *Model) refreshViewport() {
	if !m.loaded {
		return
	}
	for !m.reachedTop && m.viewport.YOffset < m.height {
		rendered := m.renderMessage(m.topIdx)
		oldTotal := m.viewport.TotalLineCount()

		m.renderedBlock = rendered + m.renderedBlock
		m.viewport.SetContent(m.renderedBlock)

		linesAdded := m.viewport.TotalLineCount() - oldTotal
		m.viewport.YOffset += linesAdded
		m.topIdx--
		if m.topIdx < 0 {
			m.reachedTop = true
			m.topIdx = 0
		}
	}
}

func (m *Model) calculateWidth(termWidth int) int {
	w := m.chatWindowWidth
	if termWidth > 0 && termWidth < w {
		w = termWidth
	}
	return w
}

// Init initializes the history model.
func (m *Model) Init() tea.Cmd {
	return m.pollBus()
}

func (m *Model) pollBus() tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-m.bus.UIUpdates()
		if !ok {
			return nil
		}
		return ev
	}
}

// Update handles messages for scrolling and viewport updates.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	cmds := make([]tea.Cmd, 0, 1)

	switch msg := msg.(type) {
	case domain.HistoryEvent:
		m.messages = msg.Messages
		m.displays = msg.ToolDisplays
		m.initializeContent()
		return m, m.pollBus()

	case domain.DoneEvent:
		m.loaded = true
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.bus.SendAction(domain.StopAction{})
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		newWidth := m.calculateWidth(msg.Width)
		if newWidth == m.width && msg.Height == m.height {
			// Still update viewport for mouse/scrolling consistency
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

		if newWidth != m.width {
			m.width = newWidth
			renderWidth := m.width - gutterWidth
			m.renderer = ui.NewGlamourRenderer(renderWidth, m.isDark)
			m.syncBuilder()
			m.renderedMessages = make(map[int]string)
		}
		m.height = msg.Height
		m.viewport.Width = m.width
		m.viewport.Height = m.height
		if m.loaded {
			m.initializeContent()
		}
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	m.refreshViewport()

	return m, tea.Batch(cmds...)
}

// View renders the history viewport.
func (m *Model) View() string {
	if !m.loaded && len(m.messages) == 0 {
		return ""
	}
	return m.viewport.View()
}

func toIntSafe(n uintptr) (int, error) {
	if uint64(n) > uint64(math.MaxInt) {
		return 0, fmt.Errorf("value %d overflows int", n)
	}
	return int(n), nil
}
