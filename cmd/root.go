package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
 
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
	"github.com/Cyclone1070/iav/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
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
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		appState, err := state.Load()
		if err != nil {
			return err
		}

		if len(args) == 0 {
			return cmd.Help()
		}

		input := strings.Join(args, " ")
		return runAgent(cmd.Context(), cfg, appState, input)
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

func runAgent(ctx context.Context, cfg *config.Config, appState *state.State, input string) error {
	if appState.Model == "" {
		return fmt.Errorf("No model selected. Please run 'iav model' or 'iav auth' to get started.")
	}

	pathResolver, err := buildPathResolver()
	if err != nil {
		return err
	}

	fileSystem := fs.NewOSFileSystem(cfg)
	cmdExecutor := executor.NewOSCommandExecutor(cfg)
	checksumMgr := hash.NewChecksumManager()
	todoStore := todo.NewInMemoryTodoStore()

	ignoreMatcher, err := git.NewIgnoreMatcher(pathResolver.Root(), fileSystem)
	if err != nil {
		ignoreMatcher = nil
	}

	tools := []domain.Tool{
		directory.NewListDirectoryTool(fileSystem, cfg, pathResolver, ignoreMatcher),
		file.NewReadFileTool(fileSystem, checksumMgr, pathResolver, cfg),
		file.NewEditFileTool(fileSystem, checksumMgr, pathResolver, cfg),
		file.NewWriteFileTool(fileSystem, checksumMgr, pathResolver, cfg),
		search.NewFindFileTool(fileSystem, cmdExecutor, cfg, pathResolver),
		search.NewSearchContentTool(fileSystem, cmdExecutor, cfg, pathResolver),
		shell.NewShellTool(fileSystem, cmdExecutor, cfg, pathResolver),
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

	llmInstance, err := llmRegistry.Get(ctx, appState.Model)
	if err != nil {
		return fmt.Errorf("get llm: %w", err)
	}

	store, err := buildSessionStore(cfg)
	if err != nil {
		return err
	}

	deps := &workflow.PromptDeps{
		Config:       cfg,
		State:        appState,
		Store:        store,
		LLM:          llmInstance,
		ToolRegistry: toolRegistry,
		Runner:       realUIRunner{},
	}

	return workflow.RunPrompt(ctx, input, deps)
}

type realUIRunner struct{}

func (realUIRunner) Run(m tea.Model) error {
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
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
