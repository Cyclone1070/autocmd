package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	stream := prompt.NewStream(ui.NewGlamourRenderer(chatWidth, true))
	thinking := prompt.NewThinkingRenderer(theme, chatWidth, ui.NewToolOutputGater(5))
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

	router := actionrouter.New()
	defer router.Close()

	cwd, _ := os.Getwd()
	pathResolver := path.NewResolver(cwd)
	fileSystem := fs.NewOSFileSystem(1024 * 1024)
	cmdExecutor := executor.NewOSCommandExecutor(fileSystem)
	checksumManager := hash.NewChecksumManager()
	taskMgr := bash.NewTaskManager(fileSystem)

	tools := []domain.Tool{
		file.NewWriteFileTool(fileSystem, checksumManager, pathResolver, 1024*1024),
		file.NewEditFileTool(fileSystem, checksumManager, pathResolver, 1024*1024),
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
	toolExecutor := agent.NewToolExecutor(registry, router)

	overflowLLM := &overflowMockLLM{}
	agentLoop := agent.NewLoop(overflowLLM, toolExecutor, 20, bus, taskMgr)

	deps := &workflow.PromptDeps{
		State:        &state.State{},
		Store:        &mockStore{},
		LLM:          overflowLLM,
		Agent:        agentLoop,
		Bus:          bus,
		Forwarder:    router,
		ToolRegistry: registry,
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

type mockStore struct{}

func (s *mockStore) Create() (*domain.Session, error)       { return &domain.Session{ID: "test"}, nil }
func (s *mockStore) Get(id string) (*domain.Session, error) { return &domain.Session{ID: id}, nil }
func (s *mockStore) Save(sess *domain.Session) error        { return nil }
func (s *mockStore) GenerateName(ctx context.Context, llm domain.LLM, target string) (string, error) {
	return "Test Session", nil
}

type overflowMockLLM struct {
	turn int
}

func (l *overflowMockLLM) ID() string                        { return "overflow-mock" }
func (l *overflowMockLLM) DisplayName() string               { return "Overflow Mock" }
func (l *overflowMockLLM) ContextWindow() int                { return 1000 }
func (l *overflowMockLLM) Model() model.ToolCallingChatModel { return l }
func (l *overflowMockLLM) ComputeTokens(ctx context.Context, msgs []*schema.Message) (int, error) {
	return 0, nil
}

func (l *overflowMockLLM) Generate(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return nil, errors.New("not implemented")
}
func (l *overflowMockLLM) BindTools(tools []*schema.ToolInfo) error { return nil }
func (l *overflowMockLLM) Bind(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return l, nil
}

func (l *overflowMockLLM) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return l, nil
}

func (l *overflowMockLLM) Stream(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	// Pace demo turns so tool transitions are readable.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(500 * time.Millisecond):
	}

	lines := make([]string, 15)
	for i := range 15 {
		lines[i] = fmt.Sprintf("Line %d: This is content for overflow testing.", i+1)
	}
	longArg := strings.Join(lines, "\n")
	cwd, _ := os.Getwd()
	target := filepath.ToSlash(filepath.Join(cwd, "demo/overflow/overflow_demo_tmp.txt"))

	steps := []struct {
		name string
		args string
	}{
		{"grep", fmt.Sprintf(`{"pattern":"package ","path":%q,"output_mode":"files_with_matches","type":"go"}`, filepath.ToSlash(filepath.Join(cwd, "internal/agent")))},
		{"glob", fmt.Sprintf(`{"pattern":"*.go","path":%q}`, filepath.ToSlash(filepath.Join(cwd, "internal")))},
		{"bash", fmt.Sprintf(`{"command":"echo overflow-demo","description":%q}`, "Running bash overflow description. "+longArg)},
		{"write_file", fmt.Sprintf(`{"file_path":"%s","content":"new content","description":%q}`, target, "Writing exactly one file with intent: "+longArg)},
		{"edit_file", fmt.Sprintf(`{"file_path":"%s","description":%q,"old_string":"new content","new_string":"changed content"}`, target, "Applying multi-line refactoring: "+longArg)},
		{"read_file", fmt.Sprintf(`{"file_path":"%s"}`, target)},
	}

	var msg *schema.Message
	if l.turn >= len(steps) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
		msg = &schema.Message{
			Role:    schema.Assistant,
			Content: "Demo finished. Each tool displayed above had at least one multi-line or extremely long argument.",
		}
	} else {
		s := steps[l.turn]
		l.turn++
		msg = &schema.Message{
			Role:    schema.Assistant,
			Content: fmt.Sprintf("Running overflow step %d: %s...", l.turn, s.name),
			ToolCalls: []schema.ToolCall{
				{
					ID:       fmt.Sprintf("overflow-%d-%s", l.turn, s.name),
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
