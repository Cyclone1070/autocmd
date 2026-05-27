package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strings"

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
	"github.com/Cyclone1070/iav/internal/tool/mcp"
	"github.com/Cyclone1070/iav/internal/tool/question"
	"github.com/Cyclone1070/iav/internal/tool/read"
	"github.com/Cyclone1070/iav/internal/tool/write"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/Cyclone1070/iav/internal/tool/service/checksum"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/Cyclone1070/iav/internal/ui/prompt"
	"github.com/Cyclone1070/iav/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var debug bool
var newSession bool

var rootCmd = &cobra.Command{
	Use:          "iav [prompt]",
	Short:        "IAV is an agentic AI coding assistant",
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
	if deps.State.Model() == "" {
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
	}

	mcpPath, err := mcp.ResolveConfigPath()
	if err == nil {
		mcpCfg, err := mcp.LoadConfigPath(mcpPath)
		if err == nil && len(mcpCfg.McpServers) > 0 {
			mcpMgr := mcp.NewManager(mcpCfg, nil, nil)
			defer func() {
				_ = mcpMgr.Close()
			}()

			fetchedTools, err := mcpMgr.Start(ctx)
			if err != nil {
				return fmt.Errorf("failed to start mcp: %w", err)
			}
			tools = append(tools, fetchedTools...)
		}
	}

	toolRegistry := tool.NewRegistry(tools)

	llmInstance, err := deps.LLMRegistry.Get(ctx, deps.State.Model())
	if err != nil {
		return withCategory(errSetup, err)
	}

	// Wiring
	bus := eventbus.New()
	defer bus.Close()
	router := actionrouter.New()
	defer router.Close()
	permissionResolver := permission.NewResolver(
		deps.Config.Tools().PermissionDefault(),
		deps.Config.Tools().ToolPermissions(),
	)
	summarizer := agent.NewSummarizer(llmInstance)
	graphRunner, err := agent.NewGraphRunner(
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
		return withCategory(errSetup, err)
	}

	themeCfg := ui.ThemeConfig{
		PrimaryColor:   ui.ToAdaptiveColor(deps.Config.UI().PrimaryColor()),
		SuccessColor:   ui.ToAdaptiveColor(deps.Config.UI().SuccessColor()),
		ErrorColor:     ui.ToAdaptiveColor(deps.Config.UI().ErrorColor()),
		MutedColor:     ui.ToAdaptiveColor(deps.Config.UI().MutedColor()),
		ShortToolBlock: deps.Config.UI().ShortToolBlock(),
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

	// Loop UI Wiring
	glamour := ui.NewGlamourRenderer(chatWidth, true)
	stream := prompt.NewStream(glamour)

	theme := ui.NewTheme(themeCfg)
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

	depsWP := &workflow.PromptDeps{
		Store:        deps.SessionStore,
		LLM:          llmInstance,
		ToolRegistry: toolRegistry,
		Agent:        graphRunner,
		Bus:          bus,
		Forwarder:    router,
		Session:      sess,
	}

	done := workflow.RunPrompt(ctx, input, depsWP)

	p := tea.NewProgram(uiModel)
	if _, err := p.Run(); err != nil {
		return withCategory(errSetup, err)
	}

	agentErr := <-done
	if agentErr != nil && !errors.Is(agentErr, context.Canceled) {
		if errors.Is(agentErr, agent.ErrModel) {
			return withCategory(errModelProvider, agentErr)
		}
		return withCategory(errAgenticLoop, agentErr)
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
