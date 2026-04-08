package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Cyclone1070/iav/internal/actionrouter"
	"github.com/Cyclone1070/iav/internal/agent"
	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/eventbus"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/Cyclone1070/iav/internal/tool"
	"github.com/Cyclone1070/iav/internal/tool/directory"
	"github.com/Cyclone1070/iav/internal/tool/file"
	"github.com/Cyclone1070/iav/internal/tool/question"
	"github.com/Cyclone1070/iav/internal/tool/search"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/Cyclone1070/iav/internal/tool/shell"
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

	// Real Tool Setup
	cwd, _ := os.Getwd()
	pathResolver := &mockPathResolver{cwd: cwd}
	fileSystem := fs.NewOSFileSystem(1024 * 1024)
	cmdExecutor := executor.NewOSCommandExecutor()

	tools := []domain.Tool{
		file.NewWriteFileTool(fileSystem, &mockChecksum{}, pathResolver, 1024*1024),
		file.NewEditFileTool(fileSystem, &mockChecksum{}, pathResolver, 1024*1024),
		file.NewReadFileTool(fileSystem, &mockChecksum{}, pathResolver),
		directory.NewListDirectoryTool(fileSystem, pathResolver, nil),
		search.NewSearchContentTool(fileSystem, cmdExecutor, pathResolver),
		search.NewFindFileTool(fileSystem, cmdExecutor, pathResolver),
		question.NewQuestionTool(),
		shell.NewShellTool(cmdExecutor, pathResolver),
	}
	registry := tool.NewRegistry(tools)
	toolExecutor := agent.NewToolExecutor(registry, router)

	// NEW: Use the real agent.Loop with a stateful MockLLM
	mockLLM := &statefulMockLLM{}
	agentLoop := agent.NewLoop(mockLLM, toolExecutor, 20, bus)

	deps := &workflow.PromptDeps{
		State:        &state.State{},
		Store:        &mockStore{},
		LLM:          mockLLM, // satisfy workflow.Deps
		Agent:        agentLoop,
		Bus:          bus,
		Forwarder:    router,
		ToolRegistry: registry,
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

type statefulMockLLM struct {
	turn int
}

func (l *statefulMockLLM) ID() string                        { return "all-tool-mock" }
func (l *statefulMockLLM) DisplayName() string               { return "All Tool Mock" }
func (l *statefulMockLLM) ContextWindow() int                { return 1000 }
func (l *statefulMockLLM) Model() model.ToolCallingChatModel { return l }

func (l *statefulMockLLM) Generate(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return nil, errors.New("not implemented")
}

func (l *statefulMockLLM) Stream(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	steps := []struct {
		name string
		args string
	}{
		{"write_file", `{"path": "temp.md", "content": "# Temp File\nInitial content.", "comment": "Creating temp file"}`},
		{"edit_file", `{"path": "temp.md", "operations": [{"before": "Initial content.", "after": "Updated via Edit Tool.", "expected_replacements": 1}], "comment": "Updating content"}`},
		{"read_file", `{"path": "temp.md"}`},
		{"list_directory", `{"path": "."}`},
		{"search_content", `{"pattern": "Updated", "path": "."}`},
		{"find_file", `{"pattern": "temp.md"}`},
		{"ask_question", `{"questions": [{"question": "Did you see the real tools working?", "options": ["Yes", "Hell yeah"]}]}`},
		{"shell", `{"command": ["rm", "temp.md"], "comment": "Cleaning up"}`},
	}

	var msg *schema.Message
	if l.turn >= len(steps) {
		// End of demo
		msg = &schema.Message{Role: schema.Assistant, Content: "\n\n### Demo Complete\n'temp.md' has been cleaned up."}
	} else {
		s := steps[l.turn]
		l.turn++

		msg = &schema.Message{
			Role:    schema.Assistant,
			Content: fmt.Sprintf("Running Step %d: %s...", l.turn, s.name),
			ToolCalls: []schema.ToolCall{
				{
					ID:       s.name + "-call",
					Function: schema.FunctionCall{Name: s.name, Arguments: s.args},
				},
			},
		}
	}

	sr, sw := schema.Pipe[*schema.Message](1)
	sw.Send(msg, nil)
	sw.Close()
	return sr, nil
}

func (l *statefulMockLLM) BindTools(tools []*schema.ToolInfo) error { return nil }

func (l *statefulMockLLM) Bind(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return l, nil
}

func (l *statefulMockLLM) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return l, nil
}

// Mocks for dependencies
type mockPathResolver struct{ cwd string }

func (r *mockPathResolver) Abs(p string) (string, error) {
	if filepath.IsAbs(p) {
		return p, nil
	}
	return filepath.Join(r.cwd, p), nil
}

func (r *mockPathResolver) Rel(p string) (string, error) {
	return filepath.Rel(r.cwd, p)
}

func (r *mockPathResolver) Root() string { return r.cwd }

type mockChecksum struct{}

func (c *mockChecksum) Compute(data []byte) string          { return "sum" }
func (c *mockChecksum) Get(path string) (string, bool)      { return "sum", true }
func (c *mockChecksum) Update(path string, checksum string) {}

type mockStore struct{}

func (s *mockStore) Create() (*domain.Session, error)       { return &domain.Session{ID: "all-tool"}, nil }
func (s *mockStore) Get(id string) (*domain.Session, error) { return &domain.Session{ID: id}, nil }
func (s *mockStore) Save(sess *domain.Session) error        { return nil }
func (s *mockStore) GenerateName(ctx context.Context, llm domain.LLM, target string) (string, error) {
	return "All Tool Demo", nil
}
