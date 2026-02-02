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
	termHeight int // For overflow indicator

	// New State tracking
	streamingMd      *StreamingMarkdown // Handles text buffering strings
	tools            []*toolState       // Ordered list of all tools (active + waiting flush)
	maxContentHeight int                // Tracks highest content height to prevent status bar jiggling

	// Keep
	thinking bool
	runState runState

	// Serial Print Queue to enforce strict output ordering and safe shutdown
	printQueue []string
	isPrinting bool
}

// msgPrintFinished is sent when a scheduled print command completes.
type msgPrintFinished struct{}

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

func newModel(cfg *config.Config) (*model, error) {
	th := newTheme(cfg.UI)
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = th.SpinnerStyle()

	// Detect terminal size
	width := cfg.UI.ChatWindowWidth
	height := 24 // default fallback
	if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		if w < width {
			width = w
		}
		height = h
	}

	// Detect cursor position to setup bottom anchor
	initialRow, err := GetCursorRow()
	// Fallback if detention fails (e.g. non-interactive): assume top of screen
	if err != nil {
		initialRow = 1
	}

	// Calculate initial available space below cursor
	// height - row = lines below.
	// We want to force padding matching this space so status bar sits at bottom.
	// We subtract 2 because the status bar function explicitly adds 2 lines of overhead (\n\n).
	spaceBelow := max(height-initialRow-2, 0)

	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize markdown renderer: %w", err)
	}

	md := NewStreamingMarkdown(r)

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
	case msg:
		return m.handleEvent(ev.Event)

	case spinner.TickMsg:
		var cmd tea.Cmd
		var newSpinner spinner.Model
		newSpinner, cmd = m.spinner.Update(ev)
		m.spinner = newSpinner

		// Note: We don't need to force render here for tool spinners because
		// in inline mode, the View() is reprinted on every update automatically by Bubble Tea framework
		// assuming standard Update/View cycle. With Altscreen/Viewport we needed it because content was cached.
		// Wait, View() IS called after every Update. Standard Bubble Tea behavior.

		return m, cmd

	case tea.KeyMsg:
		if ev.Type == tea.KeyCtrlC {
			m.runState = stateCancelled
			// Enter quitting state, triggers safe exit logic
			return m.handleDoneEvent()
		}

	case tea.WindowSizeMsg:
		m.width = min(ev.Width, m.config.UI.ChatWindowWidth)
		m.termHeight = ev.Height

		r, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(m.width),
		)
		if err != nil {
			return m, tea.Sequence(
				tea.Println(fmt.Sprintf("Fatal: glamour re-init failed: %v", err)),
				tea.Quit,
			)
		}
		m.glamour = r
		m.glamour = r
		m.streamingMd.SetRenderer(r)

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

func (m *model) handleEvent(ev domain.Event) (tea.Model, tea.Cmd) {
	switch e := ev.(type) {
	case domain.ThinkingEvent:
		m.thinking = true
		return m, m.spinner.Tick

	case domain.TextEvent:
		m.thinking = false

		// Flush any completed tools at the front of the queue
		cmds := m.flushCompletedTools()

		// Append text and get flushable blocks
		flushedBlocks, err := m.streamingMd.Append(e.Text)
		if err != nil {
			// Log error but continue?
			cmds = append(cmds, m.schedulePrint(fmt.Sprintf("Error rendering markdown: %v", err)))
		}

		for _, block := range flushedBlocks {
			cmds = append(cmds, m.schedulePrint(block))

			// Reduce maxContentHeight as content is flushed to history
			lines := strings.Count(block, "\n") + 1
			m.maxContentHeight -= lines
		}

		if m.maxContentHeight < 0 {
			m.maxContentHeight = 0
		}

		if len(cmds) > 0 {
			return m, tea.Sequence(cmds...)
		}
		return m, nil

	case domain.ToolStartEvent:
		m.thinking = false

		// Flush completed tools first (heuristic: new tool implies old ones might be done done)
		var cmds []tea.Cmd
		cmds = append(cmds, m.flushCompletedTools()...)

		// Note: We do NOT force flush text here. We rely on StreamingMarkdown logic.
		// If the text block was "Uncertain", it remains "Uncertain" until a new block starts
		// or we force flush.

		textFlush, _ := m.streamingMd.Flush()
		if textFlush != "" {
			cmds = append(cmds, m.schedulePrint(textFlush))

			// Reduce maxContentHeight
			lines := strings.Count(textFlush, "\n") + 1
			m.maxContentHeight -= lines
			if m.maxContentHeight < 0 {
				m.maxContentHeight = 0
			}
		}

		// Initialize new tool
		ts := &toolState{
			callID:  e.CallID,
			display: e.Display,
			status:  statusRunning,
		}
		m.tools = append(m.tools, ts)

		cmds = append(cmds, m.spinner.Tick)
		return m, tea.Sequence(cmds...)

	case domain.ToolStreamEvent:
		// Find tool in slice
		for _, ts := range m.tools {
			if ts.callID == e.CallID {
				ts.shellOutput.WriteString(e.Chunk)
				break
			}
		}
		return m, nil

	case domain.ToolEndEvent:
		// Mark tool as done
		for _, ts := range m.tools {
			if ts.callID == e.CallID {
				if e.Error != "" {
					ts.status = statusError
					ts.err = e.Error
				} else {
					ts.status = statusSuccess
				}
				break
			}
		}

		// Attempt flush from front
		cmds := m.flushCompletedTools()
		if len(cmds) > 0 {
			return m, tea.Sequence(cmds...)
		}
		return m, nil

	case domain.DoneEvent:
		m.runState = stateDone // Start explicit wait, but show DONE status
		return m.handleDoneEvent()
	}

	return m, nil
}

// handleDoneEvent performs final flushes and triggers the safe exit wait logic.
func (m *model) handleDoneEvent() (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// 1. Flush pending text
	textFlush, _ := m.streamingMd.Flush()
	if textFlush != "" {
		cmds = append(cmds, m.schedulePrint(strings.TrimRight(textFlush, "\n")))
	}

	// 2. Flush remaining tools
	for _, ts := range m.tools {
		cmds = append(cmds, m.schedulePrint(m.viewTool(ts)))
	}
	m.tools = nil // Clear state

	// 3. Queue FINAL Status Bar print
	// We want this to be the very last thing, so we queue it.
	// The PrintQueue guarantees it appears AFTER the above flushes.
	finalStatus := strings.TrimPrefix(m.statusBar(), "\n")
	cmds = append(cmds, m.schedulePrint(finalStatus))

	return m, tea.Sequence(cmds...)
}

// schedulePrint adds content to the queue and attempts to process it.
func (m *model) schedulePrint(content string) tea.Cmd {
	if content == "" {
		return nil
	}
	m.printQueue = append(m.printQueue, content)
	return m.processQueue()
}

// processQueue checks if we can print the next item.
func (m *model) processQueue() tea.Cmd {
	// If already printing or nothing to print, wait.
	if m.isPrinting || len(m.printQueue) == 0 {
		return nil
	}

	// Pop
	content := m.printQueue[0]
	m.printQueue = m.printQueue[1:]

	// Lock
	m.isPrinting = true

	// Exec
	return tea.Sequence(
		tea.Println(content),
		func() tea.Msg { return msgPrintFinished{} },
	)
}

// flushCompletedTools checks the front of the tool queue.
// If the first tool is complete, it flushes it and repeats for subsequent tools.
// Returns a list of tea.Println commands.
func (m *model) flushCompletedTools() []tea.Cmd {
	var cmds []tea.Cmd

	// While list is not empty AND first item is not running
	for len(m.tools) > 0 && m.tools[0].status != statusRunning {
		// Pop front
		tool := m.tools[0]
		m.tools = m.tools[1:]

		// Create flush command using tracked helper
		output := m.viewTool(tool)
		cmds = append(cmds, m.schedulePrint(output))

		// Adjust maxContentHeight downwards because this content is moving to history
		// We count lines in the output (plus 2 for the double-newline join separation that would have been there)
		lines := strings.Count(output, "\n") + 1

		// Also account for the separation that View() adds between tools (\n\n)
		// If there were multiple tools, each had separation.
		// Use a heuristic: just decrement by the tool height for now.
		if m.maxContentHeight > lines {
			m.maxContentHeight -= lines
		} else {
			m.maxContentHeight = 0
		}
	}

	return cmds
}

func (m *model) View() string {
	// When done or cancelled, we've already flushed everything via tea.Println
	// Return empty to prevent duplicate rendering and extra whitespace
	if m.runState == stateDone || m.runState == stateCancelled || m.runState == stateQuitting {
		return ""
	}

	var contentParts []string

	// 1. Pending markdown (last uncertain block)
	if pending := m.streamingMd.Pending(); pending != "" {
		pending = m.truncateWithIndicator(pending)
		contentParts = append(contentParts, pending)
	}

	// 2. All remaining tools (running + waiting-to-flush)
	for _, t := range m.tools {
		contentParts = append(contentParts, m.viewTool(t))
	}

	// Join content parts
	content := strings.Join(contentParts, "\n")

	// Calculate current content height (line count)
	currentHeight := 0
	if content != "" {
		// Fix: Use strictly newline count.
		// "hello" (0 newlines) occupies 1 visual line but 0 vertical lines relative to start.
		// "hello\n" (1 newline) occupies 1 vertical line.
		currentHeight = strings.Count(content, "\n")
	}

	// Update max content height (only grows, never shrinks during session)
	if currentHeight > m.maxContentHeight {
		m.maxContentHeight = currentHeight
	}

	// Add padding to maintain consistent height (prevents status bar jiggling)
	paddingLines := m.maxContentHeight - currentHeight
	var padding string
	if paddingLines > 0 {
		padding = strings.Repeat("\n", paddingLines)
	}

	// Build final view: content + padding + status bar
	// Status bar now has builtin \n\n prefix
	statusBar := m.statusBar()

	if content == "" {
		// No content, just padding + status bar
		return padding + statusBar
	}

	// Content + Padding + StatusBar
	// Note: content usually does not end with \n\n unless multiple tools
	return content + padding + statusBar
}

// Overflow indicator - show only bottom portion if too tall
func (m *model) truncateWithIndicator(content string) string {
	lines := strings.Split(content, "\n")
	// Calculate available space: total height - tool buffer - status
	// But we don't know tool buffer height easily without rendering.
	// We'll use a conservative heuristic: leave 5 lines.
	maxLines := max(m.termHeight-5,
		// Minimum visibility
		5)

	if len(lines) <= maxLines {
		return content
	}

	overflow := len(lines) - maxLines
	visible := lines[overflow:]
	header := fmt.Sprintf("\n  ↑ (%d lines temporarily truncated)", overflow)
	return header + "\n" + strings.Join(visible, "\n")
}

func (m *model) statusBar() string {
	// Determine theme function based on state
	var themeFunc func(string) string
	switch m.runState {
	case stateDone:
		themeFunc = m.theme.Success
	case stateCancelled:
		themeFunc = m.theme.Error
	default:
		themeFunc = m.theme.Primary
	}

	// Hardcoded context window info for now
	contextInfo := themeFunc("Context: 42%")

	var left string
	switch m.runState {
	case stateDone:
		left = fmt.Sprintf("%s %s", themeFunc("✓"), themeFunc("Done"))
	case stateCancelled:
		left = fmt.Sprintf("%s %s", themeFunc("✗"), themeFunc("Cancelled"))
	default:
		status := "Generating"
		if m.thinking {
			status = "Thinking"
		}
		// Spinner is styled separately in newModel but we can't easily change its color dynamically
		// without recreating dependencies or updating style.
		// Providing the status text in theme color.
		left = fmt.Sprintf("%s %s", m.spinner.View(), themeFunc(status))
	}

	// Calculate padding to right-align context info
	// Approximate: width - left length - right length
	// Since ANSI codes mess up length, use a fixed padding approach
	padding := max(
		// rough estimate leaving room for both sides
		m.width-20, 1)

	// Hardcode 1 empty line (\n\n) above status bar
	return "\n\n" + fmt.Sprintf("%s%*s", left, padding, contextInfo)
}
