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
	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/Cyclone1070/iav/internal/state"
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
		bootstrapFS := fs.NewOSFileSystem(-1)

		configMgr := config.NewManager(bootstrapFS)
		cfg, err := configMgr.Load()
		if err != nil {
			return err
		}

		stateMgr := state.NewManager(bootstrapFS)
		appState, err := stateMgr.Load()
		if err != nil {
			return err
		}

		if len(args) == 0 {
			return cmd.Help()
		}

		input := strings.Join(args, " ")
		return runAgent(cmd.Context(), bootstrapFS, cfg, stateMgr, appState, input)
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

func runAgent(ctx context.Context, bootstrapFS fs.FileSystem, cfg *config.Config, stateMgr *state.Manager, appState *state.State, input string) error {
	if appState.Model() == "" {
		return fmt.Errorf("No model selected. Please run 'iav model' or 'iav auth' to get started.")
	}

	pathResolver, err := buildPathResolver()
	if err != nil {
		return err
	}

	fileSystem := fs.NewOSFileSystem(cfg.Tools().MaxFileSize())
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
		file.NewEditFileTool(fileSystem, checksumMgr, pathResolver, cfg.Tools().MaxFileSize()),
		file.NewWriteFileTool(fileSystem, checksumMgr, pathResolver, cfg.Tools().MaxFileSize()),
		search.NewFindFileTool(fileSystem, cmdExecutor, pathResolver),
		search.NewSearchContentTool(fileSystem, cmdExecutor, pathResolver),
		shell.NewShellTool(fileSystem, cmdExecutor, time.Duration(cfg.Tools().DefaultShellTimeout())*time.Second, pathResolver),
		todo.NewReadTodosTool(todoStore),
		todo.NewWriteTodosTool(todoStore),
	}
	toolRegistry := tool.NewRegistry(tools)

	authMgr, err := buildAuthManager(cfg)
	if err != nil {
		return err
	}

	// Resolve credential based on provider ID (e.g., "google")
	llmRegistry := buildLLMRegistry(authMgr)

	llmInstance, err := llmRegistry.Get(ctx, appState.Model())
	if err != nil {
		return fmt.Errorf("get llm: %w", err)
	}

	store, err := buildSessionStore(cfg, bootstrapFS)
	if err != nil {
		return err
	}

	// Wiring
	bus := workflow.NewEventBus()
	defer bus.Close()
	agentLoop := agent.NewLoop(llmInstance, toolRegistry, cfg.Tools().MaxIterations(), bus)

	themeCfg := ui.ThemeConfig{
		PrimaryColor: ui.ToAdaptiveColor(cfg.UI().PrimaryColor()),
		SuccessColor: ui.ToAdaptiveColor(cfg.UI().SuccessColor()),
		ErrorColor:   ui.ToAdaptiveColor(cfg.UI().ErrorColor()),
		MutedColor:   ui.ToAdaptiveColor(cfg.UI().MutedColor()),
		ShortToolbox: cfg.UI().ShortToolbox(),
	}
	// Calculate width and height capping at terminal size
	chatWidth := cfg.UI().ChatWindowWidth()
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
	tooling := ui.NewToolRenderer(theme, chatWidth, cfg.UI().ShellOutputHeight())
	
	uiModel := loop.NewModel(
		bus,
		thinking,
		tooling,
		spinner,
		stream,
		animator,
		chatWidth,
		loop.WithTermHeight(termHeight),
	)

	deps := &workflow.PromptDeps{
		State:        appState,
		Store:        store,
		LLM:          llmInstance,
		ToolRegistry: toolRegistry,
		Agent:        agentLoop,
		Bus:          bus,
	}

	done := workflow.RunPrompt(ctx, input, deps)

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
