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
	thinkingHeight = 5
	toolingHeight  = 12
	demoTokenLimit = 1000
	stepDelay      = 250 * time.Millisecond
	phaseGapDelay  = 200 * time.Millisecond
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

	chatWidth := cfg.ChatWindowWidth()
	termHeight := 0
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
	stream := prompt.NewStream(ui.NewGlamourRenderer(chatWidth, true))
	thinking := prompt.NewThinkingRenderer(theme, chatWidth, ui.NewToolOutputGater(thinkingHeight))
	tooling := ui.NewToolRenderer(theme, chatWidth, ui.NewToolOutputGater(toolingHeight))
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

	done := workflow.RunPrompt(context.Background(), "thinking demo", deps)
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
	sleep := func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(stepDelay):
			return nil
		}
	}

	sendThoughtChunks := func(chunks []string) error {
		a.bus.SendUIUpdate(domain.ThinkingEvent{})
		for _, chunk := range chunks {
			if err := sleep(); err != nil {
				return err
			}
			a.bus.SendUIUpdate(domain.TextEvent{
				Text:      chunk,
				IsThought: true,
			})
		}
		return sleep()
	}

	firstThinking := []string{
		"Inspecting recent user request and planning UI updates. ",
		"I should keep this short enough to preview in the live thought content block, ",
		"but long enough to show wrapping and truncation behavior while the spinner is active.",
		"Inspecting recent user request and planning UI updates. ",
		"I should keep this short enough to preview in the live thought content block, ",
		"but long enough to show wrapping and truncation behavior while the spinner is active.",
		"Inspecting recent user request and planning UI updates. ",
		"I should keep this short enough to preview in the live thought content block, ",
		"but long enough to show wrapping and truncation behavior while the spinner is active.",
	}
	if err := sendThoughtChunks(firstThinking); err != nil {
		return err
	}

	time.Sleep(phaseGapDelay)

	a.bus.SendUIUpdate(domain.TextEvent{
		Text: "Thought phase one finished. Next we run one tool call.\n\n",
	})

	callID := "tool-1"
	tool := domain.NewBashDisplay("Inspect files", "rg -n \"thinking\" internal/ui/prompt", "/Users/mac/repos/iav", "")
	a.bus.SendUIUpdate(domain.ToolStartEvent{CallID: callID, Display: tool})
	if err := sleep(); err != nil {
		return err
	}
	if err := sleep(); err != nil {
		return err
	}
	a.bus.SendUIUpdate(domain.ToolEndEvent{
		CallID: callID,
		Display: domain.NewBashDisplay(
			"Inspect files",
			"rg -n \"thinking\" internal/ui/prompt",
			"/Users/mac/repos/iav",
			"",
		),
	})

	secondThinking := []string{
		"Now composing final response after the tool output. ",
		"Ensuring we keep streamed thought content visible during thinking, ",
		"and collapse back to only the final duration line once the phase completes.",
		"Now composing final response after the tool output. ",
		"Now composing final response after the tool output. ",
		"Ensuring we keep streamed thought content visible during thinking, ",
		"and collapse back to only the final duration line once the phase completes.",
		"Ensuring we keep streamed thought content visible during thinking, ",
		"and collapse back to only the final duration line once the phase completes.",
	}
	if err := sendThoughtChunks(secondThinking); err != nil {
		return err
	}

	a.bus.SendUIUpdate(domain.TextEvent{
		Text: "Done. This demo streamed thought -> tool -> thought with 250ms cadence.\n",
	})
	return nil
}

type mockStore struct{}

func (s *mockStore) Create() (*domain.Session, error) {
	return &domain.Session{ID: "thinking-demo"}, nil
}
func (s *mockStore) Get(id string) (*domain.Session, error) { return &domain.Session{ID: id}, nil }
func (s *mockStore) Save(sess *domain.Session) error        { return nil }
func (s *mockStore) GenerateName(ctx context.Context, llm domain.LLM, target string) (string, error) {
	return "Thinking Demo", nil
}

type mockLLM struct{}

func (l *mockLLM) ID() string                        { return "mock" }
func (l *mockLLM) DisplayName() string               { return "Mock LLM" }
func (l *mockLLM) ContextWindow() int                { return demoTokenLimit }
func (l *mockLLM) Model() model.ToolCallingChatModel { return nil }
func (l *mockLLM) ComputeTokens(ctx context.Context, msgs []*schema.Message) (int, error) {
	return 0, nil
}

type mockRegistry struct{}

func (r *mockRegistry) Definitions() []*schema.ToolInfo     { return nil }
func (r *mockRegistry) Get(name string) (domain.Tool, bool) { return nil, false }
