package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Cyclone1070/iav/internal/agent"
	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/Cyclone1070/iav/internal/session"
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
	"github.com/Cyclone1070/iav/internal/ui/loop"
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

		if len(args) == 0 {
			return cmd.Help()
		}

		input := strings.Join(args, " ")
		return runAgent(cfg, input)
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug logging to ~/.config/iav/debug.log")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runAgent(cfg *config.Config, input string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	llmRegistry, err := buildLLMRegistry(ctx, cfg)
	if err != nil {
		return err
	}

	appState, err := state.Load()
	if err != nil {
		return err
	}

	llmInstance, err := llmRegistry.Get(ctx, appState.Model)
	if err != nil {
		return err
	}

	store, err := buildSessionStore(cfg)
	if err != nil {
		return err
	}

	var sessionID string
	// For the main agent command, we always use the current session from state.
	// (Unless we add session selection flags later).
	sessionID = appState.CurrentSessionID

	var sess *domain.Session
	if sessionID == "" {
		sess, err = store.Create()
		if err != nil {
			return err
		}
		appState.CurrentSessionID = sess.ID
		if err := state.Save(appState); err != nil {
			slog.Warn("failed to save state", "error", err)
		}
	} else {
		sess, err = store.Get(sessionID)
		if err != nil {
			return err
		}
		if appState.CurrentSessionID != sess.ID {
			appState.CurrentSessionID = sess.ID
			if err := state.Save(appState); err != nil {
				slog.Warn("failed to save state", "error", err)
			}
		}
	}

	events := make(chan domain.Event, 100)
	broker := agent.NewEventBroker(events)
	agentLoop := agent.NewLoop(llmInstance, toolRegistry, cfg, broker)

	var namingWg sync.WaitGroup
	// Trigger auto-naming if this is a new session (no name yet)
	if sess.Name == "" {
		namingWg.Add(1) // Increment counter for the goroutine
		go func() {
			defer namingWg.Done() // Decrement counter when goroutine finishes
			name, err := session.GenerateName(ctx, llmInstance, sess, input)
			if err == nil {
				sess.Name = name
			}
		}()
	}

	m := loop.NewModel(events, cfg.UI)

	done := make(chan error, 1)
	go func() {
		err := agentLoop.Run(ctx, sess, input)
		// Wait for auto-naming to finish or be canceled before saving
		namingWg.Wait()

		// 1. DATA SAFETY FIRST: Save history immediately after loop returns.
		// This ensures history is persisted even if the UI/Broker shutdown hangs.
		_ = store.Save(sess)

		// 2. UI CLEANUP: Signal the UI to finish up.
		broker.Close()
		select {
		case events <- domain.DoneEvent{}:
		default:
			// UI already gone or buffer full, skip sentinel
		}
		close(events)
		done <- err
	}()

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "UI failed: %v\n", err)
	}

	cancel()
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

	logDir := filepath.Join(home, ".config", config.ConfigDir)
	_ = os.MkdirAll(logDir, 0755)
	logPath := filepath.Join(logDir, "debug.log")

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}

	logger := slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)
	slog.Debug("Logging initialized")
}
