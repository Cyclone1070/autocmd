package main

import (
	"log"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/tool/directory"
	"github.com/Cyclone1070/iav/internal/tool/file"
	"github.com/Cyclone1070/iav/internal/tool/search"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/Cyclone1070/iav/internal/tool/service/fs"
	"github.com/Cyclone1070/iav/internal/tool/service/git"
	"github.com/Cyclone1070/iav/internal/tool/service/hash"
	"github.com/Cyclone1070/iav/internal/tool/service/path"
	"github.com/Cyclone1070/iav/internal/tool/shell"
	"github.com/Cyclone1070/iav/internal/tool/todo"
	"github.com/Cyclone1070/iav/internal/workflow/toolmanager"
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

	listDirTool := directory.NewListDirectoryTool(fileSystem, cfg, pathResolver, ignoreMatcher)
	readFileTool := file.NewReadFileTool(fileSystem, checksumMgr, pathResolver, cfg)
	editFileTool := file.NewEditFileTool(fileSystem, checksumMgr, pathResolver, cfg)
	writeFileTool := file.NewWriteFileTool(fileSystem, checksumMgr, pathResolver, cfg)
	findFileTool := search.NewFindFileTool(fileSystem, cmdExecutor, cfg, pathResolver)
	searchContentTool := search.NewSearchContentTool(fileSystem, cmdExecutor, cfg, pathResolver)
	shellTool := shell.NewShellTool(fileSystem, cmdExecutor, cfg, pathResolver)
	readTodosTool := todo.NewReadTodosTool(todoStore)
	writeTodosTool := todo.NewWriteTodosTool(todoStore)

	tm := toolmanager.NewToolManager(
		listDirTool,
		readFileTool,
		editFileTool,
		writeFileTool,
		findFileTool,
		searchContentTool,
		shellTool,
		readTodosTool,
		writeTodosTool,
	)

	_ = tm

	log.Println("All tools wired successfully!")
}
