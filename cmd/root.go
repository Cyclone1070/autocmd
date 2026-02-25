package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/Cyclone1070/iav/internal/agent"
	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/Cyclone1070/iav/internal/session"
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
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "iav [prompt]",
	Short: "IAV is an agentic AI coding assistant",
	Args:  cobra.ArbitraryArgs,
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

	llmInstance, err := llmRegistry.Get(ctx, cfg.Model)
	if err != nil {
		return err
	}

	store, err := buildSessionStore(cfg)
	if err != nil {
		return err
	}

	var sess *domain.Session
	if cfg.Session.CurrentSessionID != "" {
		sess, err = store.Get(cfg.Session.CurrentSessionID)
		if err != nil {
			// If session not found, create a new one
			sess, err = store.Create()
		}
	} else {
		sess, err = store.Create()
	}

	if err != nil {
		return err
	}

	// Persist the session ID if it changed or was empty
	if cfg.Session.CurrentSessionID != sess.ID {
		cfg.Session.CurrentSessionID = sess.ID
		_ = config.Save(cfg)
	}

	events := make(chan domain.Event, 100)
	loop := agent.NewLoop(llmInstance, toolRegistry, cfg, events)

	var namingWg sync.WaitGroup
	// Trigger auto-naming if this is a new session (no name yet)
	if sess.Name == "" {
		namingWg.Add(1)
		go func() {
			defer namingWg.Done()
			name, err := session.GenerateName(context.Background(), llmInstance, input)
			if err == nil {
				_ = store.Rename(sess.ID, name)
			}
		}()
	}

	m := ui.NewModel(events, cfg.UI)

	done := make(chan error, 1)
	go func() {
		err := loop.Run(ctx, sess, input)
		close(events)
		_ = store.Save(sess)
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

	namingWg.Wait()
	return nil
}
