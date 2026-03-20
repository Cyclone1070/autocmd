package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/Cyclone1070/iav/internal/ui/prompt"
	"github.com/Cyclone1070/iav/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

func main() {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	bus := workflow.NewEventBus()
	cfg := config.DefaultConfig().UI()
	cfg.SetChatWindowWidth(80)

	// Calculate width and height capping at terminal size
	chatWidth := cfg.ChatWindowWidth()
	termHeight := 0 // Fallback
	if width, height, err := term.GetSize(int(os.Stdout.Fd())); err == nil && width > 0 {
		if chatWidth <= 0 || width < chatWidth {
			chatWidth = width
		}
		termHeight = height
	}

	themeCfg := ui.ThemeConfig{
		PrimaryColor: ui.ToAdaptiveColor(cfg.PrimaryColor()),
		SuccessColor: ui.ToAdaptiveColor(cfg.SuccessColor()),
		ErrorColor:   ui.ToAdaptiveColor(cfg.ErrorColor()),
		MutedColor:   ui.ToAdaptiveColor(cfg.MutedColor()),
		ShortToolbox: cfg.ShortToolbox(),
	}
	theme := ui.NewTheme(themeCfg)
	s := prompt.NewStream(ui.NewGlamourRenderer(chatWidth, true))
	anim := prompt.NewTextAnimator(4)
	thinking := prompt.NewThinkingRenderer(theme)
	tooling := ui.NewToolRenderer(theme, chatWidth, ui.NewToolOutputGater(12))
	spinner := ui.NewSpinnerRenderer(lipgloss.NewStyle().Foreground(theme.PrimaryColor()))

	m := prompt.NewModel(
		bus,
		thinking,
		tooling,
		spinner,
		theme,
		s,
		anim,
		ui.NewTruncatingGater(termHeight),
		chatWidth,
	)

	deps := &workflow.PromptDeps{
		State:        &state.State{},
		Store:        &mockStore{},
		LLM:          &mockLLM{},
		Agent:        &mockAgent{bus: bus},
		Bus:          bus,
		ToolRegistry: &mockRegistry{},
	}

	done := workflow.RunPrompt(context.Background(), "", deps)

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running UI: %v\n", err)
		os.Exit(1)
	}

	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		fmt.Printf("Error running workflow: %v\n", err)
		os.Exit(1)
	}
}

type mockAgent struct {
	bus *workflow.EventBus
}

func (a *mockAgent) Run(ctx context.Context, sess *domain.Session, input string) error {
	runSuite := func(name string, display1, display2, display3 domain.ToolDisplay) error {
		a.bus.SendUIUpdate(domain.TextEvent{Text: fmt.Sprintf("### SUITE: %s\n\n", name)})

		// Start all three
		a.bus.SendUIUpdate(domain.ToolStartEvent{CallID: name + "-1", Display: display1})
		a.bus.SendUIUpdate(domain.ToolStartEvent{CallID: name + "-2", Display: display2})
		a.bus.SendUIUpdate(domain.ToolStartEvent{CallID: name + "-3", Display: display3})

		// For STRING suite, keep original order:
		//   2 (0.5s), 3 (1.5s, error), 1 (3s).
		// For DIFF suite, make 1 finish first, 3 error second, 2 last.
		if name == "DIFF" {
			// 1 (0.5s), 3 (1.5s, error), 2 (3s)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(500 * time.Millisecond):
				a.bus.SendUIUpdate(domain.ToolEndEvent{CallID: name + "-1"})
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1000 * time.Millisecond):
				a.bus.SendUIUpdate(domain.ToolEndEvent{CallID: name + "-3", Error: "operation failed: middle tool error"})
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1500 * time.Millisecond):
				a.bus.SendUIUpdate(domain.ToolEndEvent{CallID: name + "-2"})
			}
		} else {
			// Default: 2 (0.5s), 3 (1.5s, error), 1 (3s)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(500 * time.Millisecond):
				a.bus.SendUIUpdate(domain.ToolEndEvent{CallID: name + "-2"})
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1000 * time.Millisecond):
				a.bus.SendUIUpdate(domain.ToolEndEvent{CallID: name + "-3", Error: "operation failed: middle tool error"})
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1500 * time.Millisecond):
				a.bus.SendUIUpdate(domain.ToolEndEvent{CallID: name + "-1"})
			}
		}
		return nil
	}

	a.bus.SendUIUpdate(domain.TextEvent{Text: "Starting Test Program This Is Some Filler Lines Just To Make It A Lil Bit Longer\n\n"})

	// 1. String Suite
	if err := runSuite("STRING",
		domain.NewStringDisplay("String 1 (Slow)"),
		domain.NewStringDisplay("String 2 (Fast)"),
		domain.NewStringDisplay("String 3 (Medium/Fail)")); err != nil {
		return err
	}

	// 2. Diff Suite
	if err := runSuite("DIFF",
		domain.NewDiffDisplay("Updating file.txt", "Edit file.txt", 1, 1, "- old\n+ new"),
		domain.NewDiffDisplay("Fixing fast.txt", "Edit fast.txt", 1, 1, "- fast\n+ gone"),
		domain.NewDiffDisplay("Fixing med.txt", "Edit med.txt", 1, 1, "- error here\n+ failed")); err != nil {
		return err
	}

	// 3. Shell Suite (with more streaming)
	a.bus.SendUIUpdate(domain.TextEvent{Text: "### SUITE: SHELL (Heavy Streaming)\n\n"})
	a.bus.SendUIUpdate(domain.ToolStartEvent{CallID: "SHELL-1", Display: domain.NewShellDisplay("Slow Shell", "slow-cmd", nil, nil)})
	a.bus.SendUIUpdate(domain.ToolStartEvent{CallID: "SHELL-2", Display: domain.NewShellDisplay("Fast Shell", "fast-cmd", nil, nil)})
	a.bus.SendUIUpdate(domain.ToolStartEvent{CallID: "SHELL-3", Display: domain.NewShellDisplay("Medium Shell (Fail)", "med-cmd", nil, nil)})

	// Heavy streaming
	for i := 1; i <= 20; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
			if i <= 15 {
				a.bus.SendUIUpdate(domain.ToolStreamEvent{CallID: "SHELL-2", Chunk: fmt.Sprintf("Fast output line %d - working quickly...\n", i)})
			}
			if i <= 20 {
				a.bus.SendUIUpdate(domain.ToolStreamEvent{CallID: "SHELL-1", Chunk: fmt.Sprintf("Slow output line %d - taking its time...\n", i)})
			}
			if i <= 18 {
				a.bus.SendUIUpdate(domain.ToolStreamEvent{CallID: "SHELL-3", Chunk: fmt.Sprintf("Med output line %d - about to crash...\n", i)})
			}
		}
	}

	// Finishes
	a.bus.SendUIUpdate(domain.ToolEndEvent{CallID: "SHELL-2"}) // Fast
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(1000 * time.Millisecond):
		a.bus.SendUIUpdate(domain.ToolEndEvent{CallID: "SHELL-3", Error: "exit status 1: middle tool error"}) // Medium/Fail
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(1000 * time.Millisecond):
		a.bus.SendUIUpdate(domain.ToolEndEvent{CallID: "SHELL-1"}) // Slow
	}

	return nil
}

type mockStore struct{}

func (s *mockStore) Create() (*domain.Session, error)       { return &domain.Session{ID: "test"}, nil }
func (s *mockStore) Get(id string) (*domain.Session, error) { return &domain.Session{ID: id}, nil }
func (s *mockStore) Save(sess *domain.Session) error        { return nil }
func (s *mockStore) GenerateName(ctx context.Context, llm domain.LLM, target string) (string, error) {
	return "Test Session", nil
}

type mockLLM struct{}

func (l *mockLLM) ID() string          { return "mock" }
func (l *mockLLM) DisplayName() string { return "Mock LLM" }
func (l *mockLLM) ContextWindow() int  { return 1000 }
func (l *mockLLM) ComputeTokens(ctx context.Context, msgs domain.Messages) (int, error) {
	return 0, nil
}

func (l *mockLLM) Stream(ctx context.Context, msgs domain.Messages, tools []domain.Declaration) (domain.Stream, error) {
	return nil, nil
}

type mockRegistry struct{}

func (r *mockRegistry) Declarations() []domain.Declaration  { return nil }
func (r *mockRegistry) Get(name string) (domain.Tool, bool) { return nil, false }
