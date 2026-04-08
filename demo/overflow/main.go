package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/eventbus"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/Cyclone1070/iav/internal/tool/file"
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
	animator := prompt.NewTextAnimator(3)
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
		animator,
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
	// Initialize real tools with mock deps
	fs := &mockDeps{}
	exec := &mockDeps{}
	res := &mockDeps{}

	searchTool := search.NewSearchContentTool(fs, exec, res)
	findTool := search.NewFindFileTool(fs, exec, res)
	readTool := file.NewReadFileTool(fs, fs, res)
	writeTool := file.NewWriteFileTool(fs, fs, res, 1024*1024)
	editTool := file.NewEditFileTool(fs, fs, res, 1024*1024)
	shellTool := shell.NewShellTool(res, fs)

	// 15-line argument (simulating Go code or large config)
	lines := make([]string, 15)
	for i := 0; i < 15; i++ {
		lines[i] = fmt.Sprintf("Line %d: This is content for overflow testing.", i+1)
	}
	longArg := strings.Join(lines, "\n")

	// 1. search_content Overflow
	argsObj := map[string]string{"pattern": longArg, "path": "internal/agent"}
	argsData, _ := json.Marshal(argsObj)
	inv, _ := searchTool.Prepare(string(argsData))
	var searchEnd domain.ToolDisplay
	if inv != nil {
		searchEnd = inv.Display()
		a.bus.SendUIUpdate(domain.ToolStartEvent{
			CallID:   "search-overflow",
			Display:  inv.Display(),
		})
	}
	time.Sleep(200 * time.Millisecond)
	a.bus.SendUIUpdate(domain.ToolEndEvent{CallID: "search-overflow", Display: searchEnd})

	// 2. find_file Overflow
	findArgsObj := map[string]string{"pattern": longArg, "path": "internal"}
	findArgsData, _ := json.Marshal(findArgsObj)
	findInv, _ := findTool.Prepare(string(findArgsData))
	var findEnd domain.ToolDisplay
	if findInv != nil {
		findEnd = findInv.Display()
		a.bus.SendUIUpdate(domain.ToolStartEvent{
			CallID:   "find-overflow",
			Display:  findInv.Display(),
		})
	}
	time.Sleep(200 * time.Millisecond)
	a.bus.SendUIUpdate(domain.ToolEndEvent{CallID: "find-overflow", Display: findEnd})

	// 3. shell Overflow
	shellArgsObj := map[string]any{
		"command": []string{"sh", "-c", "echo '" + longArg + "'"},
		"comment": "Running a multi-line echo for overflow testing. " + longArg,
	}
	shellArgsData, _ := json.Marshal(shellArgsObj)
	shellInv, _ := shellTool.Prepare(string(shellArgsData))
	var shellEnd domain.ToolDisplay
	if shellInv != nil {
		shellEnd = shellInv.Display()
		a.bus.SendUIUpdate(domain.ToolStartEvent{
			CallID:   "shell-overflow",
			Display:  shellInv.Display(),
		})
	}
	time.Sleep(100 * time.Millisecond)
	a.bus.SendUIUpdate(domain.ToolStreamEvent{
		CallID: "shell-overflow",
		Chunk:  longArg,
	})
	time.Sleep(200 * time.Millisecond)
	if sd, ok := shellEnd.(domain.ShellDisplay); ok {
		sd.CapturedOutput = longArg
		shellEnd = sd
	}
	a.bus.SendUIUpdate(domain.ToolEndEvent{CallID: "shell-overflow", Display: shellEnd})

	// 4. write_file Overflow
	writeArgsObj := map[string]string{
		"path":    "some/extremely/deep/nested/path/to/main.go",
		"content": "new content",
		"comment": "Writing exactly one file with intent: " + longArg,
	}
	writeArgsData, _ := json.Marshal(writeArgsObj)
	writeInv, _ := writeTool.Prepare(string(writeArgsData))
	var writeEnd domain.ToolDisplay
	if writeInv != nil {
		writeEnd = writeInv.Display()
		a.bus.SendUIUpdate(domain.ToolStartEvent{
			CallID:   "write-overflow",
			Display:  writeInv.Display(),
		})
	}
	time.Sleep(200 * time.Millisecond)
	a.bus.SendUIUpdate(domain.ToolEndEvent{CallID: "write-overflow", Display: writeEnd})

	// 5. edit_file Overflow
	editArgsObj := map[string]any{
		"path":    "some/extremely/deep/nested/path/to/main.go",
		"comment": "Applying multi-line refactoring: " + longArg,
		"operations": []map[string]any{
			{"before": "Line ", "after": "CHANGED Line ", "expected_replacements": 15},
		},
	}
	editArgsData, _ := json.Marshal(editArgsObj)
	editInv, err := editTool.Prepare(string(editArgsData))
	if err != nil {
		fmt.Printf("Edit Prepare Error: %v\n", err)
	}
	var editEnd domain.ToolDisplay
	if editInv != nil {
		editEnd = editInv.Display()
		a.bus.SendUIUpdate(domain.ToolStartEvent{
			CallID:   "edit-overflow",
			Display:  editInv.Display(),
		})
	}
	time.Sleep(200 * time.Millisecond)
	a.bus.SendUIUpdate(domain.ToolEndEvent{CallID: "edit-overflow", Display: editEnd})

	time.Sleep(200 * time.Millisecond)

	// 7. read_file (Full Path Verification)
	readArgsObj := map[string]string{"path": "some/extremely/deep/nested/path/to/main.go"}
	readArgsData, _ := json.Marshal(readArgsObj)
	readInv, _ := readTool.Prepare(string(readArgsData))
	var readEnd domain.ToolDisplay
	if readInv != nil {
		readEnd = readInv.Display()
		a.bus.SendUIUpdate(domain.ToolStartEvent{
			CallID:   "read-overflow",
			Display:  readInv.Display(),
		})
	}
	time.Sleep(200 * time.Millisecond)
	a.bus.SendUIUpdate(domain.ToolEndEvent{CallID: "read-overflow", Display: readEnd})

	a.bus.SendUIUpdate(domain.TextEvent{Text: "Demo finished. Each tool displayed above had at least one multi-line or extremely long argument."})

	return nil
}

type mockStore struct{}

func (s *mockStore) Create() (*domain.Session, error)       { return &domain.Session{ID: "test"}, nil }
func (s *mockStore) Get(id string) (*domain.Session, error) { return &domain.Session{ID: id}, nil }
func (s *mockStore) Save(sess *domain.Session) error       { return nil }
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

func (r *mockRegistry) Definitions() []*schema.ToolInfo    { return nil }
func (r *mockRegistry) Get(name string) (domain.Tool, bool) { return nil, false }

// mockDeps satisfies multiple interfaces for tools
type mockDeps struct{}

func (m *mockDeps) Stat(path string) (os.FileInfo, error) {
	isDir := !strings.HasSuffix(path, ".go")
	return &mockFileInfo{isDir: isDir}, nil
}

type mockFileInfo struct {
	os.FileInfo
	isDir bool
}

func (m *mockFileInfo) IsDir() bool        { return m.isDir }
func (m *mockFileInfo) Mode() os.FileMode { return 0644 }
func (m *mockDeps) Abs(path string) (string, error) { return "/abs/" + path, nil }
func (m *mockDeps) Root() string                  { return "/abs" }
func (m *mockDeps) Rel(path string) (string, error) { return path, nil }
func (m *mockDeps) Run(ctx context.Context, cmd []string, dir string, env []string) (*executor.Result, error) {
	return &executor.Result{ExitCode: 0}, nil
}
func (m *mockDeps) RunStreaming(ctx context.Context, cmd []string, dir string, env []string) (*executor.StreamingCmd, error) {
	return &executor.StreamingCmd{}, nil
}
func (m *mockDeps) ReadFile(path string) ([]byte, error) {
	lines := make([]string, 15)
	for i := 0; i < 15; i++ {
		lines[i] = fmt.Sprintf("Line %d: old", i+1)
	}
	return []byte(strings.Join(lines, "\n")), nil
}
func (m *mockDeps) Compute(data []byte) string           { return "hash" }
func (m *mockDeps) Get(path string) (string, bool)       { return "hash", true }
func (m *mockDeps) Update(path, checksum string)         {}
func (m *mockDeps) ReadEnv(path string) ([]string, error) { return nil, nil }
func (m *mockDeps) WriteEnv(path string, env []string) error { return nil }
func (m *mockDeps) WriteFileAtomic(path string, content []byte, perm os.FileMode) error {
	return nil
}
func (m *mockDeps) EnsureDirs(path string) error { return nil }
func (m *mockDeps) ListDir(path string) ([]os.DirEntry, error) {
	return []os.DirEntry{}, nil
}
func (m *mockDeps) Mode() os.FileMode { return 0644 }
