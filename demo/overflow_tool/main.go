package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
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
	"github.com/Cyclone1070/iav/internal/tool/edit"
	"github.com/Cyclone1070/iav/internal/tool/glob"
	"github.com/Cyclone1070/iav/internal/tool/grep"
	"github.com/Cyclone1070/iav/internal/tool/question"
	"github.com/Cyclone1070/iav/internal/tool/read"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/Cyclone1070/iav/internal/tool/service/checksum"
	"github.com/Cyclone1070/iav/internal/tool/service/path"
	"github.com/Cyclone1070/iav/internal/tool/write"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/Cyclone1070/iav/internal/ui/prompt"
	"github.com/Cyclone1070/iav/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"golang.org/x/term"
)

const (
	defaultChatWidth = 80
	thinkingHeight   = 5
	toolingHeight    = 12
	demoTokenLimit   = 5000
	maxFileSize      = 1024 * 1024
	agentIterations  = 20
	turnDelay        = 450 * time.Millisecond
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

	cwd, _ := os.Getwd()
	pathResolver := path.NewResolver(cwd)
	fileSystem := fs.NewOSFileSystem(maxFileSize)
	cmdExecutor := executor.NewOSCommandExecutor(fileSystem)
	checksumManager := checksum.NewManager(nil, nil)

	taskMgr := bash.NewTaskManager(fileSystem)
	tools := []einotool.BaseTool{
		write.NewTool(fileSystem, checksumManager, pathResolver, maxFileSize),
		edit.NewTool(fileSystem, checksumManager, pathResolver, maxFileSize),
		read.NewTool(fileSystem, checksumManager, pathResolver),
		grep.NewTool(fileSystem, cmdExecutor, pathResolver),
		glob.NewTool(fileSystem, cmdExecutor, pathResolver),
		question.NewTool(),
		bash.NewTool(fileSystem, cmdExecutor, pathResolver, taskMgr),
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
	mockLLM := &overflowMockLLM{}
	agentLoop, err := agent.NewGraphRunner(mockLLM, registry, router, agentIterations, bus, taskMgr, nil, permissionResolver)
	if err != nil {
		return fmt.Errorf("create graph runner: %w", err)
	}

	deps := &workflow.PromptDeps{
		State:        &state.State{},
		Store:        &mockStore{},
		LLM:          mockLLM,
		Agent:        agentLoop,
		Bus:          bus,
		Forwarder:    router,
		ToolRegistry: registry,
	}

	done := workflow.RunPrompt(context.Background(), "overflow demo", deps)

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running UI: %w", err)
	}

	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("error running workflow: %w", err)
	}
	return nil
}

type overflowMockLLM struct {
	turn int
}

func (l *overflowMockLLM) ID() string                        { return "overflow-tool-mock" }
func (l *overflowMockLLM) DisplayName() string               { return "Overflow Tool Mock" }
func (l *overflowMockLLM) ContextWindow() int                { return demoTokenLimit }
func (l *overflowMockLLM) Model() model.ToolCallingChatModel { return l }

func (l *overflowMockLLM) Generate(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return nil, errors.New("not implemented")
}

func (l *overflowMockLLM) Stream(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(turnDelay):
	}

	cwd, _ := os.Getwd()
	targetFile := filepath.ToSlash(filepath.Join(cwd, "overflow.txt"))

	var longLines []string
	for i := 1; i <= 60; i++ {
		longLines = append(longLines, fmt.Sprintf("Line %d: This is the original content of the file.", i))
	}
	longContent := strings.Join(longLines, "\\n")

	// Create a subset for the edit tool to replace
	var oldLines []string
	var newLines []string
	for i := 20; i <= 50; i++ {
		oldLines = append(oldLines, fmt.Sprintf("Line %d: This is the original content of the file.", i))
		newLines = append(newLines, fmt.Sprintf("Line %d: THIS CONTENT HAS BEEN UPDATED WITH A LARGE DIFF BLOCK.", i))
	}
	oldString := strings.Join(oldLines, "\\n")
	newString := strings.Join(newLines, "\\n")

	var options []string
	for i := 1; i <= 20; i++ {
		options = append(options, fmt.Sprintf("Option %d: Select this to confirm scrolling works", i))
	}
	optionsJSON := `["` + strings.Join(options, `", "`) + `"]`

	steps := []struct {
		name string
		args string
	}{
		{"bash", fmt.Sprintf(`{"command":"printf '%s'","description":"Raw long bash echo"}`, longContent)},
		{"write_file", fmt.Sprintf(`{"file_path":"%s","content":"%s","description":"Writing a long file"}`, targetFile, longContent)},
		{"edit_file", fmt.Sprintf(`{"file_path":"%s","old_string":"%s","new_string":"%s","description":"Applying a large diff"}`, targetFile, oldString, newString)},
		{"bash", `{"command":"seq 1 100","description":"Generating 100 lines of bash output"}`},
		{"ask_question", fmt.Sprintf(`{"questions": [{"question": "All previous tools are now un-truncated. You can scroll up to see the echo, the large diff, and the 100 lines of seq. Which option do you choose?", "options": %s}]}`, optionsJSON)},
		{"bash", `{"command":"rm overflow.txt","description":"Cleaning up the generated file"}`},
	}

	var msg *schema.Message
	if l.turn >= len(steps) {
		msg = &schema.Message{Role: schema.Assistant, Content: "\n\n### Overflow Demo Complete\nYou have tested scrolling and un-truncation."}
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

func (l *overflowMockLLM) BindTools(tools []*schema.ToolInfo) error { return nil }
func (l *overflowMockLLM) Bind(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return l, nil
}

func (l *overflowMockLLM) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return l, nil
}

type mockStore struct{}

func (s *mockStore) Create() (*domain.Session, error) {
	return &domain.Session{ID: "overflow-tool"}, nil
}
func (s *mockStore) Get(id string) (*domain.Session, error) { return &domain.Session{ID: id}, nil }
func (s *mockStore) Save(sess *domain.Session) error        { return nil }
func (s *mockStore) GenerateName(ctx context.Context, llm domain.LLM, target string) (string, error) {
	return "Overflow Tool Demo", nil
}
