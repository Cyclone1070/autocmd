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
	"github.com/Cyclone1070/iav/internal/ui/prompt"
	"github.com/Cyclone1070/iav/internal/eventbus"
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
	stream := prompt.NewStream(ui.NewGlamourRenderer(chatWidth, true))
	thinking := prompt.NewThinkingRenderer(theme)
	tooling := ui.NewToolRenderer(theme, chatWidth, ui.NewToolOutputGater(12))
	spinner := ui.NewSpinnerRenderer(lipgloss.NewStyle().Foreground(theme.PrimaryColor()))

	m := prompt.NewModel(
		bus,
		thinking,
		tooling,
		spinner,
		theme,
		stream,
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

	if err := <-done; err != nil {
		fmt.Printf("Error running workflow: %v\n", err)
		os.Exit(1)
	}
}

type mockAgent struct {
	bus *eventbus.EventBus
}

func (a *mockAgent) Run(ctx context.Context, sess *domain.Session, input string) error {
	blocks := []string{
		"# H1 Header - Should be uppercase\n\n",
		"## H2 Header\n\n",
		"### H3 Header\n\n",
		"This is a paragraph with **bold** and *italic* and `inline code`.\n\n",
		"Here is a list:\n- Item 1\n- Item 2\n- Item 3\n\n",
		"1. Ordered item 1\n2. Ordered item 2\n\n",
		"> This is a blockquote.\n> It can have multiple lines. It can also be very long. Sometimes it can be 2 or 3 lines even.\n\n",
		"```go\nfunc hello() {\n    fmt.Println(\"Hello, World!\")\n}\n```\n\n",
		"---\n\n", // HR
		"| Table | Header |\n|-------|--------|\n| Row 1 | Cell 1 |\n| Row 2 | Cell 2 |\n\n",
		"Task list:\n- [x] Done\n- [ ] Todo",
	}

	for _, b := range blocks {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
			a.bus.SendUIUpdate(domain.TextEvent{Text: b})
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

func (l *mockLLM) ID() string          { return "mock" }
func (l *mockLLM) DisplayName() string { return "Mock LLM" }
func (l *mockLLM) ContextWindow() int  { return 1000 }
func (l *mockLLM) ComputeTokens(ctx context.Context, msgs []*schema.Message) (int, error) {
	return 0, nil
}

func (l *mockLLM) Model() model.ToolCallingChatModel {
	return nil
}


type mockRegistry struct{}

func (r *mockRegistry) Definitions() []*schema.ToolInfo  { return nil }
func (r *mockRegistry) Get(name string) (domain.Tool, bool) { return nil, false }
