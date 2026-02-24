package main

import (
	"context"
	"log"
	"os"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/Cyclone1070/iav/internal/llm"
	"github.com/Cyclone1070/iav/internal/llm/google"
	"github.com/Cyclone1070/iav/internal/session"
	"github.com/Cyclone1070/iav/internal/tool"
	"github.com/Cyclone1070/iav/internal/tool/directory"
	"github.com/Cyclone1070/iav/internal/tool/file"
	"github.com/Cyclone1070/iav/internal/tool/search"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/Cyclone1070/iav/internal/tool/service/git"
	"github.com/Cyclone1070/iav/internal/tool/service/hash"
	"github.com/Cyclone1070/iav/internal/tool/service/path"
	"github.com/Cyclone1070/iav/internal/tool/shell"
	"github.com/Cyclone1070/iav/internal/tool/todo"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/Cyclone1070/iav/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	cfg := config.DefaultConfig()
	workspaceRoot := "."

	canonicalRoot, err := path.CanonicaliseRoot(workspaceRoot)
	if err != nil {
		log.Fatalf("Invalid workspace root: %v", err)
	}

	pathResolver := path.NewResolver(canonicalRoot)
	fileSystem := fs.NewOSFileSystem(cfg)
	cmdExecutor := executor.NewOSCommandExecutor(cfg)
	checksumMgr := hash.NewChecksumManager()
	todoStore := todo.NewInMemoryTodoStore()

	ignoreMatcher, err := git.NewIgnoreMatcher(canonicalRoot, fileSystem)
	if err != nil {
		log.Printf("Warning: gitignore matcher failed: %v", err)
		ignoreMatcher = nil
	}

	// Construct all tools (composition root)
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

	// Create tool registry (pure storage)
	toolRegistry := tool.NewRegistry(tools)

	// Create dependencies
	store := session.NewStore(cfg, fileSystem)
	events := make(chan domain.Event, 100)

	// Create model registry
	googleProvider, err := google.NewProvider(context.Background(), os.Getenv("GEMINI_API_KEY"))
	if err != nil {
		log.Fatalf("Failed to create google provider: %v", err)
	}

	llmRegistry := llm.NewRegistry(googleProvider)

	// Create UI Model
	m := ui.NewModel(events, cfg.UI)

	wf := workflow.NewWorkflow(llmRegistry, toolRegistry, store, cfg, events)

	// Run application
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := "Hello, list the current directory"
	if len(os.Args) > 1 {
		input = os.Args[1]
	}

	// Run workflow in background
	wfDone := make(chan error, 1)
	go func() {
		err := wf.Run(ctx, input)
		// Cleanup
		close(events)
		wfDone <- err
	}()

	// Block on TUI. This returns when the user quits or an error occurs.
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		log.Printf("UI failed: %v", err)
	}

	// Signal workflow to stop. This is necessary (not redundant with defer cancel)
	// because we must cancel BEFORE waiting on wfDone, otherwise deadlock.
	cancel()

	// Wait for workflow cleanup
	if err := <-wfDone; err != nil && err != context.Canceled {
		log.Printf("Workflow failed: %v", err)
	}
}
