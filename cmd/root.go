package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	"github.com/Cyclone1070/iav/internal/tool/file"
	"github.com/Cyclone1070/iav/internal/tool/question"
	"github.com/Cyclone1070/iav/internal/tool/search"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/Cyclone1070/iav/internal/tool/service/hash"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/Cyclone1070/iav/internal/ui/prompt"
	"github.com/Cyclone1070/iav/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var debug bool

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
			return wrapForUser(withCategory(ErrBootstrap, err))
		}

		if len(args) == 0 {
			return cmd.Help()
		}

		input := strings.Join(args, " ")
		return wrapForUser(runAgent(cmd.Context(), deps, input))
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

func runAgent(ctx context.Context, deps *Deps, input string) error {
	if deps.State.Model() == "" {
		return ErrNoModelSelected
	}

	pathResolver, err := buildPathResolver()
	if err != nil {
		return withCategory(ErrWorkspaceUnavailable, err)
	}

	fileSystem := fs.NewOSFileSystem(deps.Config.Tools().MaxFileSize())
	cmdExecutor := executor.NewOSCommandExecutor(fileSystem)
	checksumMgr := hash.NewChecksumManager()

	taskMgr := bash.NewTaskManager(fileSystem)

	tools := []domain.Tool{
		file.NewReadFileTool(fileSystem, checksumMgr, pathResolver),
		file.NewEditFileTool(fileSystem, checksumMgr, pathResolver, deps.Config.Tools().MaxFileSize()),
		file.NewWriteFileTool(fileSystem, checksumMgr, pathResolver, deps.Config.Tools().MaxFileSize()),
		search.NewGlobTool(fileSystem, cmdExecutor, pathResolver),
		search.NewGrepTool(fileSystem, cmdExecutor, pathResolver),
		bash.NewBashTool(fileSystem, cmdExecutor, pathResolver, taskMgr),
		bash.NewSleepTool(taskMgr),
		bash.NewTaskListTool(taskMgr),
		bash.NewTaskStopTool(taskMgr),
		question.NewQuestionTool(),
	}
	toolRegistry := tool.NewRegistry(tools)

	llmInstance, err := deps.LLMRegistry.Get(ctx, deps.State.Model())
	if err != nil {
		return withCategory(ErrModelInitialization, err)
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
	agentExecutor := agent.NewToolExecutor(toolRegistry, router, permissionResolver)
	agentLoop := agent.NewLoop(llmInstance, agentExecutor, deps.Config.Tools().MaxIterations(), bus, taskMgr)

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
	if width, height, err := term.GetSize(int(os.Stdout.Fd())); err == nil && width > 0 {
		if chatWidth <= 0 || width < chatWidth {
			chatWidth = width
		}
		termHeight = height
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
		State:        deps.State,
		Store:        deps.SessionStore,
		LLM:          llmInstance,
		ToolRegistry: toolRegistry,
		Agent:        agentLoop,
		Bus:          bus,
		Forwarder:    router,
	}

	done := workflow.RunPrompt(ctx, input, depsWP)

	p := tea.NewProgram(uiModel)
	if _, err := p.Run(); err != nil {
		return withCategory(ErrUIRuntime, err)
	}

	agentErr := <-done
	if agentErr != nil && !errors.Is(agentErr, context.Canceled) {
		if errors.Is(agentErr, agent.ErrModelAuth) {
			return withCategory(ErrModelAuth, agentErr)
		}
		if errors.Is(agentErr, agent.ErrModelBackend) {
			return withCategory(ErrModelBackend, agentErr)
		}
		return agentErr
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
