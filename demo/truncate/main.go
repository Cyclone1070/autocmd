package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/eventbus"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/Cyclone1070/iav/internal/ui/prompt"
	"github.com/Cyclone1070/iav/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"golang.org/x/term"
)

const (
	defaultChatWidth  = 80
	thinkingHeight    = 5
	toolingHeight     = 12
	demoTokenLimit    = 1000
	truncateLoopLimit = 40
	initialDelay      = 350 * time.Millisecond
	loopDelay         = 300 * time.Millisecond
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("Fatal error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	bus := eventbus.New()
	cfg := config.DefaultConfig().UI()
	cfg.SetChatWindowWidth(defaultChatWidth)

	// Calculate width and height capping at terminal size.
	chatWidth := cfg.ChatWindowWidth()
	termHeight := 0 // Fallback.
	fd := os.Stdout.Fd()
	if fd <= math.MaxInt {
		if width, height, err := term.GetSize(int(fd)); err == nil && width > 0 {
			if chatWidth <= 0 || width < chatWidth {
				chatWidth = width
			}
			termHeight = height
		}
	}

	themeCfg := ui.ThemeConfig{
		PrimaryColor:   ui.ToAdaptiveColor(cfg.PrimaryColor()),
		SuccessColor:   ui.ToAdaptiveColor(cfg.SuccessColor()),
		ErrorColor:     ui.ToAdaptiveColor(cfg.ErrorColor()),
		MutedColor:     ui.ToAdaptiveColor(cfg.MutedColor()),
		ShortToolBlock: cfg.ShortToolBlock(),
	}
	theme := ui.NewTheme(themeCfg)
	s := prompt.NewStream(ui.NewGlamourRenderer(chatWidth, true))
	thinking := prompt.NewThinkingRenderer(theme, chatWidth, ui.NewToolOutputGater(thinkingHeight))
	tooling := ui.NewToolRenderer(theme, chatWidth, ui.NewToolOutputGater(toolingHeight))
	spinner := ui.NewSpinnerRenderer(lipgloss.NewStyle().Foreground(theme.PrimaryColor()))

	m := prompt.NewModel(
		bus,
		thinking,
		tooling,
		spinner,
		theme,
		s,
		ui.NewTruncatingGater(termHeight),
		chatWidth,
	)

	deps := &workflow.PromptDeps{
		State:        &state.State{},
		Store:        &mockStore{},
		LLM:          &mockLLM{},
		Agent:        &mockAgent{bus: bus},
		Bus:          bus,
	}

	done := workflow.RunPrompt(context.Background(), "", deps)

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running UI: %w", err)
	}

	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("error running workflow: %w", err)
	}
	return nil
}

type mockAgent struct {
	bus *eventbus.EventBus
}

func (a *mockAgent) Run(ctx context.Context, sess *domain.Session, input string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(initialDelay):
	}
	a.bus.SendUIUpdate(domain.TextEvent{Text: "This is a test of the truncation feature. The following lines should be truncated if they exceed the terminal width.\n\n"})
	for i := 1; i <= truncateLoopLimit; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			a.bus.SendUIUpdate(domain.TextEvent{Text: fmt.Sprintf("Line %d: This is a repeated line for truncation testing it's gonna be quite long to overflow my goated terminal.\n", i)})
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(loopDelay):
		}
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

func (l *mockLLM) ID() string                        { return "mock" }
func (l *mockLLM) DisplayName() string               { return "Mock LLM" }
func (l *mockLLM) ContextWindow() int                { return demoTokenLimit }
func (l *mockLLM) Model() model.ToolCallingChatModel { return nil }

func (l *mockLLM) ComputeTokens(ctx context.Context, msgs []*schema.Message) (int, error) {
	return 0, nil
}

