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
	"github.com/Cyclone1070/iav/demo/demoutil"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/Cyclone1070/iav/internal/ui/prompt"
	"github.com/Cyclone1070/iav/internal/eventbus"
	"github.com/Cyclone1070/iav/internal/workflow"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

func main() {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	bus := eventbus.New()
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
	anim := prompt.NewTextAnimator(3)
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
	bus *eventbus.EventBus
}

// toolDisplayWithError copies the display and sets Error for demo ToolEndEvent (matches executor baking).
func toolDisplayWithError(d domain.ToolDisplay, msg string) domain.ToolDisplay {
	switch x := d.(type) {
	case domain.StringDisplay:
		x.Error = msg
		return x
	case domain.DiffDisplay:
		x.Error = msg
		return x
	case domain.BashDisplay:
		x.Error = msg
		return x
	default:
		return d
	}
}

func (a *mockAgent) Run(ctx context.Context, sess *domain.Session, input string) error {
	tt := demoutil.NewToolTracker(a.bus)
	defer tt.FlushOpenCancelled()

	runSuite := func(name string, display1, display2, display3 domain.ToolDisplay) error {
		a.bus.SendUIUpdate(domain.TextEvent{Text: fmt.Sprintf("### SUITE: %s\n\n", name)})

		// Start all three
		tt.Start(name+"-1", display1)
		tt.Start(name+"-2", display2)
		tt.Start(name+"-3", display3)

		// For STRING suite, keep original order:
		//   2 (0.5s), 3 (1.5s, error), 1 (3s).
		// For DIFF suite, make 1 finish first, 3 error second, 2 last.
		if name == "DIFF" {
			// 1 (0.5s), 3 (1.5s, error), 2 (3s)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(500 * time.Millisecond):
				tt.End(name+"-1", display1)
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1000 * time.Millisecond):
				tt.End(name+"-3", toolDisplayWithError(display3, "operation failed: middle tool error"))
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1500 * time.Millisecond):
				tt.End(name+"-2", display2)
			}
		} else {
			// Default: 2 (0.5s), 3 (1.5s, error), 1 (3s)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(500 * time.Millisecond):
				tt.End(name+"-2", display2)
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1000 * time.Millisecond):
				tt.End(name+"-3", toolDisplayWithError(display3, "operation failed: middle tool error"))
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1500 * time.Millisecond):
				tt.End(name+"-1", display1)
			}
		}
		return nil
	}

	a.bus.SendUIUpdate(domain.TextEvent{Text: "Starting Test Program This Is Some Filler Lines Just To Make It A Lil Bit Longer\n\n"})

	// 1. String Suite
	if err := runSuite("STRING",
		domain.NewStringDisplay("Analyzing codebase", "String 1 (Slow)"),
		domain.NewStringDisplay("", "String 2 (Fast)"),
		domain.NewStringDisplay("", "String 3 (Medium/Fail)")); err != nil {
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
	tt.Start("SHELL-1", domain.NewBashDisplay("Slow Shell", "slow-cmd", ""))
	tt.Start("SHELL-2", domain.NewBashDisplay("Fast Shell", "fast-cmd", ""))
	tt.Start("SHELL-3", domain.NewBashDisplay("Medium Shell (Fail)", "med-cmd", ""))

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
	tt.End("SHELL-2", domain.NewBashDisplay("Fast Shell", "fast-cmd", "")) // Fast
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(1000 * time.Millisecond):
		bash3 := domain.NewBashDisplay("Medium Shell (Fail)", "med-cmd", "")
		bash3.Error = "exit status 1: middle tool error"
		tt.End("SHELL-3", bash3) // Medium/Fail
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(1000 * time.Millisecond):
		tt.End("SHELL-1", domain.NewBashDisplay("Slow Shell", "slow-cmd", "")) // Slow
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
func (l *mockLLM) Model() model.ToolCallingChatModel { return nil }

func (l *mockLLM) ComputeTokens(ctx context.Context, msgs []*schema.Message) (int, error) {
	return 0, nil
}


type mockRegistry struct{}

func (r *mockRegistry) Definitions() []*schema.ToolInfo  { return nil }
func (r *mockRegistry) Get(name string) (domain.Tool, bool) { return nil, false }
