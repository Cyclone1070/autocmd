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
		PrimaryColor:   ui.ToAdaptiveColor(cfg.PrimaryColor()),
		SuccessColor:   ui.ToAdaptiveColor(cfg.SuccessColor()),
		ErrorColor:     ui.ToAdaptiveColor(cfg.ErrorColor()),
		MutedColor:     ui.ToAdaptiveColor(cfg.MutedColor()),
		ShortToolBlock: cfg.ShortToolBlock(),
	}
	theme := ui.NewTheme(themeCfg)
	s := prompt.NewStream(ui.NewGlamourRenderer(chatWidth, true))
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

func (a *mockAgent) Run(ctx context.Context, sess *domain.Session, input string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(350 * time.Millisecond):
	}
	a.bus.SendUIUpdate(domain.TextEvent{Text: "This is a test of the truncation feature. The following lines should be truncated if they exceed the terminal width.\n\n"})
	for i := 1; i <= 40; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			a.bus.SendUIUpdate(domain.TextEvent{Text: fmt.Sprintf("Line %d: This is a repeated line for truncation testing it's gonna be quite long to overflow my goated terminal.\n", i)})
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
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
func (l *mockLLM) ContextWindow() int                { return 1000 }
func (l *mockLLM) Model() model.ToolCallingChatModel { return nil }

func (l *mockLLM) ComputeTokens(ctx context.Context, msgs []*schema.Message) (int, error) {
	return 0, nil
}

type mockRegistry struct{}

func (r *mockRegistry) Definitions() []*schema.ToolInfo     { return nil }
func (r *mockRegistry) Get(name string) (domain.Tool, bool) { return nil, false }
