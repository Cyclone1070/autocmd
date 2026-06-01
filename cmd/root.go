package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"strings"

	"github.com/Cyclone1070/autocmd/internal/actionrouter"
	"github.com/Cyclone1070/autocmd/internal/agent"
	"github.com/Cyclone1070/autocmd/internal/domain"
	"github.com/Cyclone1070/autocmd/internal/eventbus"
	"github.com/Cyclone1070/autocmd/internal/fs"
	"github.com/Cyclone1070/autocmd/internal/logging"
	"github.com/Cyclone1070/autocmd/internal/permission"
	"github.com/Cyclone1070/autocmd/internal/tool"
	"github.com/Cyclone1070/autocmd/internal/tool/bash"
	"github.com/Cyclone1070/autocmd/internal/tool/edit"
	"github.com/Cyclone1070/autocmd/internal/tool/glob"
	"github.com/Cyclone1070/autocmd/internal/tool/grep"
	"github.com/Cyclone1070/autocmd/internal/tool/mcp"
	"github.com/Cyclone1070/autocmd/internal/tool/question"
	"github.com/Cyclone1070/autocmd/internal/tool/read"
	"github.com/Cyclone1070/autocmd/internal/tool/save"
	"github.com/Cyclone1070/autocmd/internal/tool/service/checksum"
	"github.com/Cyclone1070/autocmd/internal/tool/service/executor"
	"github.com/Cyclone1070/autocmd/internal/tool/write"
	"github.com/Cyclone1070/autocmd/internal/ui"
	"github.com/Cyclone1070/autocmd/internal/ui/prompt"
	"github.com/Cyclone1070/autocmd/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	debug      bool
	newSession bool
)

var rootCmd = &cobra.Command{
	Use:          "autocmd [prompt]",
	Short:        "AutoCmd is a terminal-native AI assistant",
	Args:         cobra.ArbitraryArgs,
	SilenceUsage: true,
	PersistentPreRun: func(_ *cobra.Command, _ []string) {
		setupLogging()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := Wire()
		if err != nil {
			return wrapForUser(withCategory(errSetup, err))
		}

		if len(args) == 0 {
			return cmd.Help()
		}

		workingDir := getWorkingDir()

		if newSession {
			if _, err := workflow.CreateSession(deps.SessionStore, workingDir); err != nil {
				return wrapForUser(withCategory(errSetup, err))
			}
		}

		input := strings.Join(args, " ")

		if saved, ok := deps.CommandStore.Get(input); ok {
			return wrapForUser(runSavedCommand(cmd.Context(), saved.Command, workingDir))
		}

		return wrapForUser(runAgent(cmd.Context(), deps, input, workingDir))
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, fmt.Sprintf("Enable debug logging to ~/%s/%s/debug.log", domain.ConfigBaseDir, domain.AppName))
	rootCmd.Flags().BoolVarP(&newSession, "new", "n", false, "Start a new session for this prompt")
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runAgent(ctx context.Context, deps *Deps, input string, workingDir string) error {
	if deps.State.Model == "" {
		return withCategory(errSetup, errors.New("no model selected"))
	}

	sess, err := workflow.ResolveWorkspaceSession(deps.SessionStore, workingDir)
	if err != nil {
		return withCategory(errSetup, fmt.Errorf("failed to resolve workspace session: %w", err))
	}

	pathResolver, err := buildPathResolver()
	if err != nil {
		return withCategory(errWorkspaceUnavailable, err)
	}

	fileSystem := fs.NewOSFileSystem(deps.Config.Tools().MaxFileSize())
	cmdExecutor := executor.NewOSCommandExecutor(fileSystem)
	checksumMgr := checksum.NewManager(deps.SessionStore, sess.ID)

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
		bash.NewTaskStopAllTool(taskMgr),
		question.NewTool(),
		save.NewTool(deps.CommandStore),
	}

	// Load MCP config (file read only, no connection)
	mcpPath, mcpErr := mcp.ResolveConfigPath()
	var mcpCfg *mcp.Config
	if mcpErr == nil {
		mcpCfg, _ = mcp.LoadConfigPath(mcpPath)
	}
	hasMCP := mcpCfg != nil && len(mcpCfg.McpServers) > 0

	// Wiring
	bus := eventbus.New()
	defer bus.Close()
	router := actionrouter.New()
	defer router.Close()

	// Create MCP manager (lifecycle managed in main goroutine)
	var mcpMgr *mcp.Manager
	if hasMCP {
		mcpMgr = mcp.NewManager(mcpCfg, nil, nil)
		defer func() { _ = mcpMgr.Close() }()
		bus.SendUIUpdate(domain.MCPLoadingEvent{})
	}

	// Calculate width and height capping at terminal size
	chatWidth := deps.Config.UI().ChatWindowWidth()
	termHeight := 0 // Fallback (0 disables global truncation)
	fd, err := toIntSafe(os.Stdout.Fd())
	if err == nil && term.IsTerminal(fd) {
		if width, height, err := term.GetSize(fd); err == nil && width > 0 {
			if chatWidth <= 0 || width < chatWidth {
				chatWidth = width
			}
			termHeight = height
		}
	}
	if chatWidth <= 0 {
		chatWidth = 80
	}

	// Loop UI Wiring
	glamour := ui.NewGlamourRenderer(chatWidth, true)
	stream := prompt.NewStream(glamour)

	theme := newTheme(deps.Config.UI())
	spinner := ui.NewSpinnerRenderer(lipgloss.NewStyle().Foreground(theme.PrimaryColor()))
	thinking := prompt.NewThinkingRenderer(theme, chatWidth, ui.NewToolOutputGater(deps.Config.UI().ThinkingHeight()))
	tooling := ui.NewToolRenderer(theme, chatWidth, ui.NewToolOutputGater(deps.Config.UI().BashOutputHeight()))

	uiModel := prompt.NewModel(
		bus,
		thinking,
		tooling,
		spinner,
		theme,
		stream,
		ui.NewTruncatingGater(termHeight),
		chatWidth,
	)

	resultCh := make(chan error, 1)

	go func() {
		var mcpTools []einotool.BaseTool
		if mcpMgr != nil {
			mcpTools, _ = mcpMgr.Start(ctx)
		}

		allTools := append([]einotool.BaseTool{}, tools...)
		allTools = append(allTools, mcpTools...)
		toolRegistry := tool.NewRegistry(allTools)

		llmInstance, llmErr := deps.LLMRegistry.Get(ctx, deps.State.Model)
		if llmErr != nil {
			bus.SendUIUpdate(domain.DoneEvent{})
			resultCh <- withCategory(errSetup, llmErr)
			return
		}

		permissionResolver := permission.NewResolver(
			deps.Config.Tools().PermissionDefault(),
			deps.Config.Tools().ToolPermissions(),
		)
		summarizer := agent.NewSummarizer(llmInstance)
		graphRunner, gErr := agent.NewGraphRunner(
			llmInstance,
			toolRegistry,
			router,
			deps.Config.Tools().MaxIterations(),
			bus,
			taskMgr,
			summarizer,
			permissionResolver,
		)
		if gErr != nil {
			bus.SendUIUpdate(domain.DoneEvent{})
			resultCh <- withCategory(errSetup, gErr)
			return
		}

		depsWP := &workflow.PromptDeps{
			Store:     deps.SessionStore,
			LLM:       llmInstance,
			Agent:     graphRunner,
			Bus:       bus,
			Forwarder: router,
			Session:   sess,
		}

		done := workflow.RunPrompt(ctx, input, depsWP)
		agentErr := <-done
		resultCh <- agentErr
	}()

	p := tea.NewProgram(uiModel)
	if _, err := p.Run(); err != nil {
		return withCategory(errSetup, err)
	}

	agentErr := <-resultCh
	if agentErr != nil && !errors.Is(agentErr, context.Canceled) {
		if errors.Is(agentErr, agent.ErrModel) {
			return withCategory(errModelProvider, agentErr)
		}
		return withCategory(errAgenticLoop, agentErr)
	}

	return nil
}

func runSavedCommand(ctx context.Context, command, workingDir string) error {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	//nolint:gosec // intentional: running user-saved commands
	cmd := exec.CommandContext(ctx, shell, "-c", command)
	cmd.Dir = workingDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}
	return nil
}

func setupLogging() {
	logger, logPath, err := logging.Init(logging.Options{Debug: debug})
	if err != nil {
		fallback := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
		slog.SetDefault(fallback)
		slog.Warn("failed to initialize file logging; falling back to stderr", "error", err)
		return
	}
	slog.SetDefault(logger)
	slog.Debug("logging initialized", "path", logPath, "debug", debug)
}

func toIntSafe(n uintptr) (int, error) {
	if uint64(n) > uint64(math.MaxInt) {
		return 0, fmt.Errorf("value %d overflows int", n)
	}
	return int(n), nil
}
