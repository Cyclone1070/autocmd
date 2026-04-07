package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/Cyclone1070/iav/demo/demoutil"
	"github.com/Cyclone1070/iav/internal/actionrouter"
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

	chatWidth := cfg.ChatWindowWidth()
	termHeight := 0
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

	router := actionrouter.New()
	defer router.Close()
	tt := demoutil.NewToolTracker(bus)

	deps := &workflow.PromptDeps{
		State:     &state.State{},
		Store:     &mockStore{},
		LLM:       &mockLLM{},
		Agent:     &mockAgent{bus: bus, tt: tt, router: router, theme: theme},
		Bus:       bus,
		Forwarder: router,
	}

	done := workflow.RunPrompt(context.Background(), "demo", deps)

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

func formatAnswerSummary(t *ui.Theme, questions []domain.QuestionInfo, answers [][]string) string {
	var sb strings.Builder
	for i, q := range questions {
		if i >= len(answers) {
			continue
		}
		ans := "Not answered"
		arrow := t.Error("→")
		if len(answers[i]) > 0 {
			ans = strings.Join(answers[i], ", ")
			arrow = t.Success("→")
		}
		fmt.Fprintf(&sb, "• %s %s %s\n", q.Question, arrow, ans)
	}
	return strings.TrimSpace(sb.String())
}

func sampleQuestionDisplay() domain.QuestionDisplay {
	return domain.NewQuestionDisplay([]domain.QuestionInfo{
		{
			Question: "Pick a color for the demo banner",
			Options:  []string{"Blue", "Amber", "Violet", "Grass Green", "Blood Scarlet", "Nerd Purple"},
			Multiple: false,
		},
		{
			Question: "Select environments to deploy",
			Options:  []string{"Staging", "QA", "Production"},
			Multiple: true,
		},
		{
			Question: "Choose rollout strategy",
			Options:  []string{"Canary", "Blue/Green", "Immediate"},
			Multiple: false,
		},
	})
}

type mockAgent struct {
	bus    *eventbus.EventBus
	tt     *demoutil.ToolTracker
	router *actionrouter.Router
	theme  *ui.Theme
}

func (a *mockAgent) Run(ctx context.Context, sess *domain.Session, input string) error {
	defer a.tt.FlushOpenCancelled()
	a.bus.SendUIUpdate(domain.TextEvent{Text: "### Question tool demo\n\nUse **↑/↓** to move, **Enter** to confirm, **c** (also **i** / **o**) for a custom answer, and **Esc** to cancel.\n\n"})

	time.Sleep(300 * time.Millisecond)

	callID := "question-demo-1"
	a.tt.Start(callID, sampleQuestionDisplay())

	act, err := a.router.Wait(ctx, callID)
	if err != nil {
		return err
	}

	if qa, ok := act.(domain.QuestionAnswerAction); ok {
		end := domain.NewStringDisplay("", formatAnswerSummary(a.theme, sampleQuestionDisplay().Questions, qa.Answers))
		a.tt.End(callID, end)
	}

	return nil
}

type mockStore struct{}

func (s *mockStore) Create() (*domain.Session, error) {
	return &domain.Session{ID: "question-demo"}, nil
}
func (s *mockStore) Get(id string) (*domain.Session, error) { return &domain.Session{ID: id}, nil }
func (s *mockStore) Save(sess *domain.Session) error        { return nil }
func (s *mockStore) GenerateName(ctx context.Context, llm domain.LLM, target string) (string, error) {
	return "Question demo", nil
}

type mockLLM struct{}

func (l *mockLLM) ID() string                        { return "mock" }
func (l *mockLLM) DisplayName() string               { return "Mock LLM" }
func (l *mockLLM) ContextWindow() int                { return 1000 }
func (l *mockLLM) Model() model.ToolCallingChatModel { return nil }
func (l *mockLLM) ComputeTokens(ctx context.Context, msgs []*schema.Message) (int, error) {
	return 0, nil
}
