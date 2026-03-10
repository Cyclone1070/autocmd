package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/Cyclone1070/iav/internal/ui/loop"
	"github.com/Cyclone1070/iav/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	bus := workflow.NewEventBus()
	cfg := config.DefaultConfig()
	cfg.UI.ChatWindowWidth = 80
	m := loop.NewModel(bus, cfg.UI)

	deps := &workflow.PromptDeps{
		Config:       cfg,
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
	a.bus.SendUIUpdate(domain.TextEvent{Text: "This is a test of the truncation feature. The following lines should be truncated if they exceed the terminal width.\n\n"})
	for i := 1; i <= 40; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			a.bus.SendUIUpdate(domain.TextEvent{Text: fmt.Sprintf("Line %d: This is a repeated line for truncation testing it's gonna be quite long to overflow my goated terminal.\n", i)})
		}
	}
	return nil
}

type mockStore struct{}

func (s *mockStore) Create() (*domain.Session, error)       { return &domain.Session{ID: "test"}, nil }
func (s *mockStore) Get(id string) (*domain.Session, error) { return &domain.Session{ID: id}, nil }
func (s *mockStore) Save(sess *domain.Session) error       { return nil }

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
