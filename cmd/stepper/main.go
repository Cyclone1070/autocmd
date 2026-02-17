package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui/compose"
	"github.com/Cyclone1070/iav/internal/ui/cursor"
	"github.com/Cyclone1070/iav/internal/ui/engine"
	"github.com/Cyclone1070/iav/internal/ui/markdown"
	teapkg "github.com/Cyclone1070/iav/internal/ui/tea"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

func main() {
	cfg := config.DefaultConfig()

	// 1. Geometry Detection
	cd := cursor.NewTerminalCursorDetector(os.Stdin, os.Stdout)
	width := cfg.UI.ChatWindowWidth
	height := 24
	if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		if w < width {
			width = w
		}
		height = h
	}
	geom, err := teapkg.ResolveGeometry(cfg, cd, height)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving geometry: %v\n", err)
		os.Exit(1)
	}

	// 2. Engine Initialization
	mdRenderer, _ := markdown.NewGlamourRenderer(geom.Width)
	sm := markdown.NewStream(mdRenderer)
	state := engine.NewInitialState(geom)

	factory := func(s *spinner.Model) engine.Deps {
		deps := compose.NewEngineDeps(cfg, sm, geom.Width)
		return deps
	}

	adapter := teapkg.NewTeaModelAdapter(state, factory, teapkg.ProductionSink{})

	// 3. Events Generation
	events := generateEvents()

	// 4. Run Stepper
	m := &stepperModel{
		adapter: adapter,
		events:  events,
		index:   0,
	}

	p := tea.NewProgram(m, tea.WithOutput(os.Stdout), tea.WithInput(os.Stdin))
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running stepper: %v\n", err)
		os.Exit(1)
	}
}

type stepperModel struct {
	adapter    *teapkg.TeaModelAdapter
	events     []domain.Event
	index      int
	pendingCmd tea.Cmd
}

func (m *stepperModel) Init() tea.Cmd {
	return m.adapter.Init()
}

func (m *stepperModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyRight:
			// If we have a pending print command, execute it now.
			if m.pendingCmd != nil {
				cmd := m.pendingCmd
				m.pendingCmd = nil
				return m, cmd
			}

			// Otherwise, send the next event.
			if m.index < len(m.events) {
				ev := m.events[m.index]
				m.index++

				_, cmd := m.adapter.Update(ev)

				// Capture the command if it's a print command (IsPrinting will be true)
				if m.adapter.State.IsPrinting {
					m.pendingCmd = cmd
					return m, nil // Don't run it yet, let user step into IsPrinting state.
				}

				return m, cmd
			}
		}
	}

	// Delegate ticks and other background messages
	_, cmd := m.adapter.Update(msg)
	return m, cmd
}

func (m *stepperModel) View() string {
	view := m.adapter.View()

	// Add flushing status if printing
	if m.adapter.State.IsPrinting {
		// Append to the last non-empty line (the status bar)
		lines := strings.Split(view, "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			if strings.TrimSpace(lines[i]) != "" {
				lines[i] = lines[i] + " [FLUSHING]"
				break
			}
		}
		view = strings.Join(lines, "\n")
	}

	footer := fmt.Sprintf("\n\n(Step %d/%d) [Right Arrow to Advance] [Ctrl+C to Quit]", m.index, len(m.events))
	return view + footer
}

func generateEvents() []domain.Event {
	var evs []domain.Event
	typing := func(text string) {
		runes := []rune(text)
		for i := 0; i < len(runes); {
			chunkSize := 4
			if i+chunkSize > len(runes) {
				chunkSize = len(runes) - i
			}
			evs = append(evs, domain.TextEvent{Text: string(runes[i : i+chunkSize])})
			i += chunkSize
		}
	}

	// 1. Thinking
	evs = append(evs, domain.ThinkingEvent{})

	// 2. Intro
	typing("# Integrated Architecture Validation\n\n")
	typing("I will now demonstrate the **new inline UI** capabilities. ")
	typing("This system uses a _streaming markdown parser_ to flush content block-by-block.\n\n")

	typing("Here is the plan:\n")
	typing("1. Test fragmentation handling\n")
	typing("2. Test code block streaming\n")
	typing("3. Test concurrent tool execution\n")
	typing("4. Test overflow handling for long blocks\n\n")

	// 3. Fragmentation
	typing("### Fragmentation Test\n\n")
	typing("This sentence has **bro")
	typing("ken bold** markers and `split")
	typing(" code` formatting. ")
	typing("this is ```inline ")
	typing("code block```\n\n")

	// 4. Code Block
	typing("Now writing a Go function:\n\n")
	evs = append(evs, domain.TextEvent{Text: "```go\n"})
	codeLines := []string{
		"package main\n\n",
		"func hello() {\n",
		"    fmt.Println(\"Hello World\")\n",
		"    // Simulating complex logic...\n",
		"    time.Sleep(1 * time.Second)\n",
		"}\n",
	}
	for _, line := range codeLines {
		typing(line)
	}
	evs = append(evs, domain.TextEvent{Text: "```\n\n"})
	typing("That block should now be flushed to history.\n\n")

	// 5. Tool
	typing("### Tool Execution & Ordering\n\n")
	typing("I'll start two tools. Tool B finishes FIRST, but should wait for Tool A.\n\n")
	evs = append(evs, domain.ToolStartEvent{
		CallID:   "tool-A",
		ToolName: "long-job",
		Display:  domain.StringDisplay("Tool A: Long Running Job..."),
	})
	evs = append(evs, domain.ToolStartEvent{
		CallID:   "tool-B",
		ToolName: "fast-job",
		Display:  domain.StringDisplay("Tool B: Quick Job"),
	})
	evs = append(evs, domain.ToolEndEvent{CallID: "tool-B"})
	evs = append(evs, domain.ToolEndEvent{CallID: "tool-A"})

	// 6. Overflow
	typing("\n### Overflow Indicator Test\n\n")
	typing("Generating a VERY long block to test standard output clipping and the overflow indicator:\n\n")
	for i := 0; i < 30; i++ {
		line := fmt.Sprintf("Line %03d: This is a generated line to fill the screen and force the pending block to overflow the viewport.\n", i+1)
		typing(line)
	}
	typing("\nEnd of long block.\n\n")

	// 7. Final tool
	evs = append(evs, domain.ToolStartEvent{
		CallID:   "final-shell",
		ToolName: "shell",
		Display: domain.ShellDisplay{
			Header:  "Final Cleanup",
			Command: "rm -rf /tmp/demo",
		},
	})
	chunks := []string{"cleaning...", " removing files...", " done.\n"}
	for _, c := range chunks {
		evs = append(evs, domain.ToolStreamEvent{CallID: "final-shell", Chunk: c})
	}
	evs = append(evs, domain.ToolEndEvent{CallID: "final-shell"})

	// 8. Done
	evs = append(evs, domain.DoneEvent{})

	return evs
}
