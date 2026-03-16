package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/Cyclone1070/iav/internal/ui/loop"
	"github.com/Cyclone1070/iav/internal/workflow"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	bus := workflow.NewEventBus()
	cfg := config.DefaultConfig().UI()
	
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
	s := loop.NewStream(ui.NewGlamourRenderer(chatWidth, true))
	anim := loop.NewTextAnimator(4)
	thinking := loop.NewThinkingRenderer(theme)
	tooling := ui.NewToolRenderer(theme, chatWidth, 12)
	spinner := ui.NewSpinnerRenderer(lipgloss.NewStyle().Foreground(theme.PrimaryColor()))

	m := loop.NewModel(bus, thinking, tooling, spinner, s, anim, chatWidth, loop.WithTermHeight(termHeight))

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

	if err := <-done; err != nil {
		fmt.Printf("Error running workflow: %v\n", err)
		os.Exit(1)
	}
}

type mockAgent struct {
	bus *workflow.EventBus
}

func (a *mockAgent) Run(ctx context.Context, sess *domain.Session, input string) error {
	// 1. Text Stream
	markdown := "# UI Demo\n\nThis is a demo of the **smooth streaming** logic. It breaks down text into small chunks to simulate a real-time LLM response."
	a.bus.SendUIUpdate(domain.TextEvent{Text: markdown})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(400 * time.Millisecond):
	}

	// 2. Thinking
	a.bus.SendUIUpdate(domain.ThinkingEvent{})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(1 * time.Second):
	}

	a.bus.SendUIUpdate(domain.TextEvent{Text: "Here's a readfile tool call."})

	a.bus.SendUIUpdate(domain.ThinkingEvent{})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(1 * time.Second):
	}

	a.bus.SendUIUpdate(domain.ToolStartEvent{
		CallID:  "tool-0",
		Display: domain.NewStringDisplay("Reading main.go"),
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(1 * time.Second):
	}
	a.bus.SendUIUpdate(domain.ToolEndEvent{CallID: "tool-0"})

	a.bus.SendUIUpdate(domain.TextEvent{Text: "Now let's run some tools in parallel. Tools will be displayed in toolStart order."})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(400 * time.Millisecond):
	}

	// 3. Parallel Tool Calls (3 tools)
	a.bus.SendUIUpdate(domain.ToolStartEvent{
		CallID:  "tool-1",
		Display: domain.NewShellDisplay("Finish last", "npm list --depth=0", nil, nil),
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(400 * time.Millisecond):
	}

	a.bus.SendUIUpdate(domain.ToolStartEvent{
		CallID:  "tool-2",
		Display: domain.NewShellDisplay("Finish first", "eslint .", nil, nil),
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(200 * time.Millisecond):
		a.bus.SendUIUpdate(domain.ToolStreamEvent{CallID: "tool-2", Chunk: "All files passed linting.\n"})
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(400 * time.Millisecond):
	}

	a.bus.SendUIUpdate(domain.ToolStartEvent{
		CallID:  "tool-3",
		Display: domain.NewShellDisplay("Finish second", "go test ./...", nil, nil),
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(400 * time.Millisecond):
	}

	// 3a. Tool 2 finishes first (blocked)
	a.bus.SendUIUpdate(domain.ToolStreamEvent{CallID: "tool-2", Chunk: "All files passed linting.\n"})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(400 * time.Millisecond):
	}
	a.bus.SendUIUpdate(domain.ToolEndEvent{CallID: "tool-2"})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(1 * time.Second):
	}

	// 3b. Tool 3 finishes second (blocked)
	a.bus.SendUIUpdate(domain.ToolStreamEvent{CallID: "tool-3", Chunk: "Running tests...\nPASS\n"})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(400 * time.Millisecond):
	}
	a.bus.SendUIUpdate(domain.ToolEndEvent{CallID: "tool-3"})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(1 * time.Second):
	}

	// 3c. Tool 1 finishes last (triggers cascading flush)
	a.bus.SendUIUpdate(domain.ToolStreamEvent{CallID: "tool-1", Chunk: "Found 45 dependencies.\n"})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(400 * time.Millisecond):
	}
	a.bus.SendUIUpdate(domain.ToolEndEvent{CallID: "tool-1"})

	// 6. Final text
	a.bus.SendUIUpdate(domain.TextEvent{Text: "Refactoring complete! The UI is looking great. ✨"})

	return nil
}

type mockStore struct{}

func (s *mockStore) Create() (*domain.Session, error)       { return &domain.Session{ID: "test"}, nil }
func (s *mockStore) Get(id string) (*domain.Session, error) { return &domain.Session{ID: id}, nil }
func (s *mockStore) Save(sess *domain.Session) error       { return nil }
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

func (r *mockRegistry) Declarations() []domain.Declaration { return nil }
func (r *mockRegistry) Get(name string) (domain.Tool, bool) { return nil, false }
