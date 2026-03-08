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
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug logging to ~/.config/iav/debug.log")
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
	providerID := strings.SplitN(appState.Model, domain.ModelIDSeparator, 2)[0]
	cred := resolveCredential(authMgr, providerID)

	llmRegistry := buildLLMRegistry()
	llmInstance, err := llmRegistry.Get(ctx, appState.Model, cred)
	if err != nil {
		return fmt.Errorf("initialize LLM: %w", err)
	}

	store, err := buildSessionStore(cfg)
	if err != nil {
		return err
	}

	var sessionID string
	// For the main agent command, we always use the current session from state.
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
		namingWg.Add(1)
		go func() {
			defer namingWg.Done()
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
		namingWg.Wait()

		_ = store.Save(sess)

		broker.Close()
		select {
		case events <- domain.DoneEvent{}:
		default:
		}
		close(events)
		done <- err
	}()

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "UI failed: %v\n", err)
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
