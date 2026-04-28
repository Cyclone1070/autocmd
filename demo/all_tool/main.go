package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/Cyclone1070/iav/internal/actionrouter"
	"github.com/Cyclone1070/iav/internal/agent"
	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/eventbus"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/Cyclone1070/iav/internal/permission"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/Cyclone1070/iav/internal/tool"
	"github.com/Cyclone1070/iav/internal/tool/bash"
	"github.com/Cyclone1070/iav/internal/tool/file"
	"github.com/Cyclone1070/iav/internal/tool/question"
	"github.com/Cyclone1070/iav/internal/tool/search"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/Cyclone1070/iav/internal/tool/service/hash"
	"github.com/Cyclone1070/iav/internal/tool/service/path"
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
	defaultChatWidth    = 80
	thinkingHeight      = 5
	toolingHeight       = 12
	demoTokenLimit      = 1000
	maxFileSize         = 1024 * 1024
	agentIterations     = 20
	turnDelay           = 450 * time.Millisecond
)

func main() {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	bus := eventbus.New()
	cfg := config.DefaultConfig().UI()
	cfg.SetChatWindowWidth(defaultChatWidth)

	chatWidth := cfg.ChatWindowWidth()
	termHeight := 0
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

	router := actionrouter.New()
	defer router.Close()

	// Real Tool Setup
	cwd, _ := os.Getwd()
	pathResolver := path.NewResolver(cwd)
	fileSystem := fs.NewOSFileSystem(maxFileSize)
	cmdExecutor := executor.NewOSCommandExecutor(fileSystem)
	checksumManager := hash.NewChecksumManager()

	taskMgr := bash.NewTaskManager(fileSystem)
	tools := []domain.Tool{
		file.NewWriteFileTool(fileSystem, checksumManager, pathResolver, maxFileSize),
		file.NewEditFileTool(fileSystem, checksumManager, pathResolver, maxFileSize),
		file.NewReadFileTool(fileSystem, checksumManager, pathResolver),
		search.NewGrepTool(fileSystem, cmdExecutor, pathResolver),
		search.NewGlobTool(fileSystem, cmdExecutor, pathResolver),
		question.NewQuestionTool(),
		bash.NewBashTool(fileSystem, cmdExecutor, pathResolver, taskMgr),
		bash.NewSleepTool(taskMgr),
		bash.NewTaskListTool(taskMgr),
		bash.NewTaskStopTool(taskMgr),
	}
	registry := tool.NewRegistry(tools)
	permissionResolver := permission.NewResolver(
		"ask",
		map[string]string{
			"task_list": "allow",
			"task_stop": "allow",
		},
	)
	toolExecutor := agent.NewToolExecutor(registry, router, permissionResolver)

	// NEW: Use the real agent.Loop with a stateful MockLLM
	mockLLM := &statefulMockLLM{}
	agentLoop := agent.NewLoop(mockLLM, toolExecutor, agentIterations, bus, taskMgr)

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
func (l *statefulMockLLM) ContextWindow() int                { return demoTokenLimit }
func (l *statefulMockLLM) Model() model.ToolCallingChatModel { return l }

func (l *statefulMockLLM) Generate(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return nil, errors.New("not implemented")
}

func (l *statefulMockLLM) Stream(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	// Slow down each turn a bit so the demo doesn't feel instant.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(turnDelay):
	}

	cwd, _ := os.Getwd()
	tempFile := filepath.ToSlash(filepath.Join(cwd, "temp.md"))
	steps := []struct {
		name string
		args string
	}{
		{"write_file", fmt.Sprintf(`{"file_path":"%s","content":"# Temp File\nInitial content.","description":"Creating temp file"}`, tempFile)},
		{"edit_file", fmt.Sprintf(`{"file_path":"%s","description":"Updating content","old_string":"Initial content.","new_string":"Updated via Edit Tool."}`, tempFile)},
		{"read_file", fmt.Sprintf(`{"file_path":"%s"}`, tempFile)},
		{"grep", fmt.Sprintf(`{"pattern":"Updated","path":"%s","output_mode":"content"}`, filepath.ToSlash(cwd))},
		{"glob", fmt.Sprintf(`{"pattern":"temp.md","path":"%s"}`, filepath.ToSlash(cwd))},
		{"ask_question", `{"questions": [{"question": "Did you see the real tools working?", "options": ["Yes", "Hell yeah"]}]}`},
		{"bash", `{"command":"rm \"temp.md\"","description":"Cleaning up"}`},
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

type mockStore struct{}

func (s *mockStore) Create() (*domain.Session, error)       { return &domain.Session{ID: "all-tool"}, nil }
func (s *mockStore) Get(id string) (*domain.Session, error) { return &domain.Session{ID: id}, nil }
func (s *mockStore) Save(sess *domain.Session) error        { return nil }
func (s *mockStore) GenerateName(ctx context.Context, llm domain.LLM, target string) (string, error) {
	return "All Tool Demo", nil
}
