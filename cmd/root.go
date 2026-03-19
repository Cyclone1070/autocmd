package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Cyclone1070/iav/internal/agent"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/Cyclone1070/iav/internal/tool"
	"github.com/Cyclone1070/iav/internal/tool/directory"
	"github.com/Cyclone1070/iav/internal/tool/file"
	"github.com/Cyclone1070/iav/internal/tool/search"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/Cyclone1070/iav/internal/tool/service/git"
	"github.com/Cyclone1070/iav/internal/tool/service/hash"
	"github.com/Cyclone1070/iav/internal/tool/shell"
	"github.com/Cyclone1070/iav/internal/tool/todo"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/Cyclone1070/iav/internal/ui/loop"
	"github.com/Cyclone1070/iav/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var debug bool

var rootCmd = &cobra.Command{
	Use:   "iav [prompt]",
	Short: "IAV is an agentic AI coding assistant",
	Args:  cobra.ArbitraryArgs,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		setupLogging()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := Wire()
		if err != nil {
			return err
		}

		if len(args) == 0 {
			return cmd.Help()
		}

		input := strings.Join(args, " ")
		return runAgent(cmd.Context(), deps, input)
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, fmt.Sprintf("Enable debug logging to ~/%s/%s/debug.log", domain.ConfigBaseDir, domain.AppName))
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runAgent(ctx context.Context, deps *Deps, input string) error {
	if deps.State.Model() == "" {
		return fmt.Errorf("No model selected. Please run 'iav model' or 'iav auth' to get started.")
	}

	pathResolver, err := buildPathResolver()
	if err != nil {
		return err
	}

	fileSystem := fs.NewOSFileSystem(deps.Config.Tools().MaxFileSize())
	cmdExecutor := executor.NewOSCommandExecutor()
	checksumMgr := hash.NewChecksumManager()
	todoStore := todo.NewInMemoryTodoStore()

	ignoreMatcher, err := git.NewIgnoreMatcher(pathResolver.Root(), fileSystem)
	if err != nil {
		ignoreMatcher = nil
	}

	tools := []domain.Tool{
		directory.NewListDirectoryTool(fileSystem, pathResolver, ignoreMatcher),
		file.NewReadFileTool(fileSystem, checksumMgr, pathResolver),
		file.NewEditFileTool(fileSystem, checksumMgr, pathResolver, deps.Config.Tools().MaxFileSize()),
		file.NewWriteFileTool(fileSystem, checksumMgr, pathResolver, deps.Config.Tools().MaxFileSize()),
		search.NewFindFileTool(fileSystem, cmdExecutor, pathResolver),
		search.NewSearchContentTool(fileSystem, cmdExecutor, pathResolver),
		shell.NewShellTool(fileSystem, cmdExecutor, time.Duration(deps.Config.Tools().DefaultShellTimeout())*time.Second, pathResolver),
		todo.NewReadTodosTool(todoStore),
		todo.NewWriteTodosTool(todoStore),
	}
	toolRegistry := tool.NewRegistry(tools)

	llmInstance, err := deps.LLMRegistry.Get(ctx, deps.State.Model())
	if err != nil {
		return fmt.Errorf("get llm: %w", err)
	}

	// Wiring
	bus := workflow.NewEventBus()
	defer bus.Close()
	agentLoop := agent.NewLoop(llmInstance, toolRegistry, deps.Config.Tools().MaxIterations(), bus)

	themeCfg := ui.ThemeConfig{
		PrimaryColor: ui.ToAdaptiveColor(deps.Config.UI().PrimaryColor()),
		SuccessColor: ui.ToAdaptiveColor(deps.Config.UI().SuccessColor()),
		ErrorColor:   ui.ToAdaptiveColor(deps.Config.UI().ErrorColor()),
		MutedColor:   ui.ToAdaptiveColor(deps.Config.UI().MutedColor()),
		ShortToolbox: deps.Config.UI().ShortToolbox(),
	}
	// Calculate width and height capping at terminal size
	chatWidth := deps.Config.UI().ChatWindowWidth()
	termHeight := 0 // Fallback (0 disables global truncation)
	if width, height, err := term.GetSize(int(os.Stdout.Fd())); err == nil && width > 0 {
		if chatWidth <= 0 || width < chatWidth {
			chatWidth = width
		}
		termHeight = height
	}
	
	// Loop UI Wiring
	glamour := ui.NewGlamourRenderer(chatWidth, true)
	stream := loop.NewStream(glamour)
	animator := loop.NewTextAnimator(4) // 4 runes per tick
	
	theme := ui.NewTheme(themeCfg)
	spinner := ui.NewSpinnerRenderer(lipgloss.NewStyle().Foreground(theme.PrimaryColor()))
	thinking := loop.NewThinkingRenderer(theme)
	tooling := ui.NewToolRenderer(theme, chatWidth, ui.NewToolOutputGater(deps.Config.UI().ShellOutputHeight()))
	
	uiModel := loop.NewModel(
		bus,
		thinking,
		tooling,
		spinner,
		stream,
		animator,
		ui.NewTruncatingGater(termHeight),
		chatWidth,
	)

	depsWP := &workflow.PromptDeps{
		State:        deps.State,
		Store:        deps.SessionStore,
		LLM:          llmInstance,
		ToolRegistry: toolRegistry,
		Agent:        agentLoop,
		Bus:          bus,
	}

	done := workflow.RunPrompt(ctx, input, depsWP)

	p := tea.NewProgram(uiModel)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("UI failed: %w", err)
	}

	agentErr := <-done
	if agentErr != nil && !errors.Is(agentErr, context.Canceled) {
		return fmt.Errorf("agent failed: %w", agentErr)
	}

	return nil
}

func setupLogging() {
	if !debug {
		slog.SetDefault(slog.New(slog.DiscardHandler))
		return
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	logDir := filepath.Join(home, domain.ConfigBaseDir, domain.AppName)
	_ = os.MkdirAll(logDir, domain.DefaultDirPerm)
	logPath := filepath.Join(logDir, "debug.log")

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, domain.DefaultFilePerm)
	if err != nil {
		return
	}

	logger := slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)
	slog.Debug("Logging initialized")
}
