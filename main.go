package main

import (
	"log"

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
	"github.com/Cyclone1070/iav/internal/tool/service/path"
	"github.com/Cyclone1070/iav/internal/tool/shell"
	"github.com/Cyclone1070/iav/internal/tool/todo"
	"github.com/Cyclone1070/iav/internal/workflow"
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
	events := make(chan workflow.Event, 100)

	// TODO: Provider implementation is deferred.
	// Once implemented, uncomment the following:
	// provider := google.NewClient(os.Getenv("GEMINI_API_KEY"))
	// wf := workflow.NewWorkflow(provider, toolRegistry, store, cfg, events)
	// _ = wf

	_ = toolRegistry
	_ = store
	_ = events

	log.Println("All tools wired successfully!")
}
