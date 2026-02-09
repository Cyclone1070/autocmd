package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"golang.org/x/term"
)

type model struct {
	spinner    spinner.Model
	glamour    *glamour.TermRenderer
	theme      *theme
	config     *config.Config
	width      int
	termHeight int // Terminal height for overflow truncation

	streamingMd      *streamingMarkdown // Handles text buffering strings
	tools            []*toolState       // Ordered list of all tools (active + waiting flush)
	maxContentHeight int                // Tracks highest content height to prevent status bar jiggling

	thinking bool
	runState runState

	// Serial Print Queue to enforce strict output ordering and safe shutdown
	printQueue []printItem
	isPrinting bool
}

type toolState struct {
	callID      string
	display     domain.ToolDisplay
	status      toolStatus
	err         string
	shellOutput strings.Builder
}

type toolStatus int

const (
	statusRunning toolStatus = iota
	statusSuccess
	statusError
)

type runState int

const (
	stateRunning  runState = iota
	stateQuitting          // Waiting for prints to finish
	stateDone
	stateCancelled
)

const (
	// defaultTerminalHeight is used when term.GetSize() fails (e.g., non-TTY environment).
	defaultTerminalHeight = 24

	// statusBarOverhead accounts for the \n\n prefix added by statusBar().
	// This must stay in sync with the statusBar() implementation.
	statusBarOverhead = 2

	// Status display text
	textGenerating = "Generating"
	textThinking   = "Thinking"
	textDone       = "Done"
	textCancelled  = "Cancelled"
)

func newModel(cfg *config.Config, cd CursorDetector) (*model, error) {
	th := newTheme(cfg.UI)
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = th.SpinnerStyle()

	// Detect terminal size
	width := cfg.UI.ChatWindowWidth
	height := defaultTerminalHeight
	if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		if w < width {
			width = w
		}
		height = h
	}

	// Detect cursor position to setup bottom anchor
	initialRow, err := cd.GetCursorRow()
	// Fallback if detection fails (e.g. non-interactive): assume top of screen
	if err != nil {
		initialRow = 1
	}

	// Calculate initial available space below cursor
	// height - row = lines below.
	// We want to force padding matching this space so status bar sits at bottom.
	spaceBelow := max(height-initialRow-statusBarOverhead, 0)

	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize markdown renderer: %w", err)
	}

	md := newStreamingMarkdown(r)

	return &model{
		spinner:          s,
		glamour:          r,
		theme:            th,
		config:           cfg,
		width:            width,
		termHeight:       height,
		streamingMd:      md,
		tools:            make([]*toolState, 0),
		maxContentHeight: spaceBelow, // Initialize with available space to pin bottom
	}, nil
}

func (m *model) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m *model) Update(teaMsg tea.Msg) (tea.Model, tea.Cmd) {
	switch ev := teaMsg.(type) {
	case domain.Event:
		return m.handleEvent(ev)

	case spinner.TickMsg:
		var cmd tea.Cmd
		var newSpinner spinner.Model
		newSpinner, cmd = m.spinner.Update(ev)
		m.spinner = newSpinner

		return m, cmd

	case tea.KeyMsg:
		if ev.Type == tea.KeyCtrlC {
			m.runState = stateCancelled
			// Enter quitting state, triggers safe exit logic
			return m.handleDoneEvent()
		}

	// NOTE: We intentionally ignore tea.WindowSizeMsg.
	// Resizing mid-session would cause width inconsistency between flushed
	// (scrollback) and pending content. Width is locked at startup.

	case msgPrintFinished:
		m.isPrinting = false
		// Trigger next item in queue
		if nextCmd := m.processQueue(); nextCmd != nil {
			return m, nextCmd
		}

		// If Queue Empty AND Not Printing AND Done -> Safe Quit
		if (m.runState == stateDone || m.runState == stateCancelled) && len(m.printQueue) == 0 && !m.isPrinting {
			return m, tea.Quit
		}
	}
	return m, nil
}
