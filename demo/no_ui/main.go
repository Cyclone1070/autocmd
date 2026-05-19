package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Cyclone1070/iav/cmd"
	"github.com/Cyclone1070/iav/internal/actionrouter"
	"github.com/Cyclone1070/iav/internal/agent"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/eventbus"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/Cyclone1070/iav/internal/logging"
	"github.com/Cyclone1070/iav/internal/permission"
	"github.com/Cyclone1070/iav/internal/tool"
	"github.com/Cyclone1070/iav/internal/tool/bash"
	"github.com/Cyclone1070/iav/internal/tool/edit"
	"github.com/Cyclone1070/iav/internal/tool/glob"
	"github.com/Cyclone1070/iav/internal/tool/grep"
	"github.com/Cyclone1070/iav/internal/tool/question"
	"github.com/Cyclone1070/iav/internal/tool/read"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/Cyclone1070/iav/internal/tool/service/checksum"
	"github.com/Cyclone1070/iav/internal/tool/service/path"
	"github.com/Cyclone1070/iav/internal/tool/write"
	"github.com/Cyclone1070/iav/internal/workflow"
	einotool "github.com/cloudwego/eino/components/tool"
)

const (
	demoPrompt             = "hello mate can you yap for a bit and call some tools, it is very important that you yap not just call tools silently, don't call question tool though just normal tools"
	autoApprovePermissions = true
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	logger, logPath, err := logging.Init(logging.Options{Debug: true})
	if err != nil {
		fallback := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
		slog.SetDefault(fallback)
		slog.Warn("failed to initialize file logging; falling back to stderr", "error", err)
	} else {
		slog.SetDefault(logger)
		slog.Debug("logging initialized", "path", logPath, "debug", true)
	}

	deps, err := cmd.Wire()
	if err != nil {
		return fmt.Errorf("wire deps: %w", err)
	}
	if deps.State.Model() == "" {
		return fmt.Errorf("no model selected")
	}

	// Force a fresh session for every run.
	deps.State.SetCurrentSessionID("")
	if err := deps.State.Save(); err != nil {
		slog.Warn("failed to reset current session id", "error", err)
	}

	pathResolver, err := buildPathResolver()
	if err != nil {
		return fmt.Errorf("build path resolver: %w", err)
	}

	fileSystem := fs.NewOSFileSystem(deps.Config.Tools().MaxFileSize())
	cmdExecutor := executor.NewOSCommandExecutor(fileSystem)
	checksumMgr := checksum.NewManager(nil, nil)
	taskMgr := bash.NewTaskManager(fileSystem)

	tools := []einotool.BaseTool{
		read.NewTool(fileSystem, checksumMgr, pathResolver),
		edit.NewTool(fileSystem, checksumMgr, pathResolver, deps.Config.Tools().MaxFileSize()),
		write.NewTool(fileSystem, checksumMgr, pathResolver, deps.Config.Tools().MaxFileSize()),
		glob.NewTool(fileSystem, cmdExecutor, pathResolver),
		grep.NewTool(fileSystem, cmdExecutor, pathResolver),
		bash.NewTool(fileSystem, cmdExecutor, pathResolver, taskMgr),
		bash.NewSleepTool(taskMgr),
		bash.NewTaskListTool(taskMgr),
		bash.NewTaskStopTool(taskMgr),
		question.NewTool(),
	}
	toolRegistry := tool.NewRegistry(tools)

	llmInstance, err := deps.LLMRegistry.Get(ctx, deps.State.Model())
	if err != nil {
		return fmt.Errorf("init model: %w", err)
	}

	bus := eventbus.New()
	defer bus.Close()
	router := actionrouter.New()
	defer router.Close()
	permissionResolver := permission.NewResolver(
		deps.Config.Tools().PermissionDefault(),
		deps.Config.Tools().ToolPermissions(),
	)
	summarizer := agent.NewSummarizer(llmInstance)
	reactRunner, err := agent.NewGraphRunner(
		llmInstance,
		toolRegistry,
		router,
		deps.Config.Tools().MaxIterations(),
		bus,
		taskMgr,
		summarizer,
		permissionResolver,
	)
	if err != nil {
		return fmt.Errorf("create graph runner: %w", err)
	}

	depsWP := &workflow.PromptDeps{
		State:        deps.State,
		Store:        deps.SessionStore,
		LLM:          llmInstance,
		ToolRegistry: toolRegistry,
		Agent:        reactRunner,
		Bus:          bus,
		Forwarder:    router,
	}

	start := time.Now()
	done := workflow.RunPrompt(ctx, demoPrompt, depsWP)

	for upd := range bus.UIUpdates() {
		fmt.Println(formatSpyEventLine(time.Since(start), upd))
		if handleAutoActions(ctx, bus, upd, autoApprovePermissions) {
			fmt.Printf("t=%.3fs event=auto_approve payload=%q\n", time.Since(start).Seconds(), "permission request approved")
		}
		if _, ok := upd.(domain.DoneEvent); ok {
			break
		}
	}

	agentErr := <-done
	if agentErr != nil && !errors.Is(agentErr, context.Canceled) {
		return agentErr
	}
	return nil
}

func buildPathResolver() (*path.Resolver, error) {
	canonicalRoot, err := path.CanonicaliseRoot(path.OSFileSystem{}, ".")
	if err != nil {
		return nil, err
	}
	return path.NewResolver(canonicalRoot), nil
}

func formatSpyEventLine(elapsed time.Duration, upd domain.UIUpdate) string {
	return fmt.Sprintf("t=%.3fs event=%T payload=%+v", elapsed.Seconds(), upd, upd)
}

func handleAutoActions(_ context.Context, bus *eventbus.EventBus, upd domain.UIUpdate, autoApprove bool) bool {
	if !autoApprove {
		return false
	}
	req, ok := upd.(domain.ToolApprovalRequestEvent)
	if !ok {
		return false
	}
	bus.SendAction(domain.PermissionDecisionAction{
		CallID:   req.CallID,
		Approved: true,
	})
	return true
}
