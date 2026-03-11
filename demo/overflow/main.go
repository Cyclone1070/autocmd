package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/Cyclone1070/iav/internal/ui/loop"
	"github.com/Cyclone1070/iav/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	bus := workflow.NewEventBus()
	cfg := config.DefaultConfig().UI()
	cfg.SetChatWindowWidth(80)
	themeCfg := ui.ThemeConfig{
		PrimaryColor: ui.ToAdaptiveColor(cfg.PrimaryColor()),
		SuccessColor: ui.ToAdaptiveColor(cfg.SuccessColor()),
		ErrorColor:   ui.ToAdaptiveColor(cfg.ErrorColor()),
		MutedColor:   ui.ToAdaptiveColor(cfg.MutedColor()),
		ShortToolbox: cfg.ShortToolbox(),
	}
	m := loop.NewModel(bus, themeCfg, cfg.ChatWindowWidth())

	deps := &workflow.PromptDeps{
		State:        &state.State{},
		Store:        &mockStore{},
		LLM:          &mockLLM{},
		Runner:       &realRunner{},
		Agent:        &mockAgent{bus: bus},
		UI:           m,
		Bus:          bus,
		ToolRegistry: &mockRegistry{},
	}

	if err := workflow.RunPrompt(context.Background(), "", deps); err != nil {
		fmt.Printf("Error running workflow: %v\n", err)
		os.Exit(1)
	}
}

type mockAgent struct {
	bus *workflow.EventBus
}

func (a *mockAgent) Run(ctx context.Context, sess *domain.Session, input string) error {
	// Long strings for overflow testing
	longString := strings.Repeat("OverflowingContent_", 10)
	longHeader := "This is a very long header that will definitely exceed the eighty character limit of the tool box"
	longCommand := "sh -c 'echo \"This is a very long command line that will also definitely exceed the eighty character limit of the tool box\" && sleep 1'"
	longOutput := strings.Repeat("LongOutputLineContent_", 10) + "\n" + strings.Repeat("AnotherLongLine-", 15)

	// 1. StringDisplay Overflow
	a.bus.SendUIUpdate(domain.ToolStartEvent{
		CallID:  "string-overflow",
		Display: domain.NewStringDisplay("Short Header"),
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(500 * time.Millisecond):
		a.bus.SendUIUpdate(domain.ToolEndEvent{
			CallID: "string-overflow",
		})
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(500 * time.Millisecond):
	}

	a.bus.SendUIUpdate(domain.ToolStartEvent{
		CallID:  "string-overflow-2",
		Display: domain.NewStringDisplay(longHeader),
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(500 * time.Millisecond):
		a.bus.SendUIUpdate(domain.ToolEndEvent{
			CallID: "string-overflow-2",
		})
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(500 * time.Millisecond):
	}

	// 2. DiffDisplay Overflow
	a.bus.SendUIUpdate(domain.ToolStartEvent{
		CallID:  "diff-overflow",
		Display: domain.NewDiffDisplay(longHeader, "Edit "+longString, 1, 1, "+ "+longString+"\n- "+longString),
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(500 * time.Millisecond):
		a.bus.SendUIUpdate(domain.ToolEndEvent{CallID: "diff-overflow"})
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(500 * time.Millisecond):
	}

	// 3. ShellDisplay Overflow (All parts)
	a.bus.SendUIUpdate(domain.ToolStartEvent{
		CallID:  "shell-overflow",
		Display: domain.NewShellDisplay(longHeader, longCommand, nil, nil),
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(500 * time.Millisecond):
		a.bus.SendUIUpdate(domain.ToolStreamEvent{
			CallID: "shell-overflow",
			Chunk:  longOutput,
		})
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(500 * time.Millisecond):
		a.bus.SendUIUpdate(domain.ToolEndEvent{CallID: "shell-overflow"})
	}

	return nil
}

type mockStore struct{}

func (s *mockStore) Create() (*domain.Session, error)       { return &domain.Session{ID: "test"}, nil }
func (s *mockStore) Get(id string) (*domain.Session, error) { return &domain.Session{ID: id}, nil }
func (s *mockStore) Save(sess *domain.Session) error       { return nil }
func (s *mockStore) GenerateName(ctx context.Context, llm domain.LLM, sess *domain.Session, input string) (string, error) {
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

type realRunner struct{}

func (r *realRunner) Run(m tea.Model) error {
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}

type mockRegistry struct{}

func (r *mockRegistry) Declarations() []domain.Declaration { return nil }
func (r *mockRegistry) Get(name string) (domain.Tool, bool) { return nil, false }
