package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"regexp"
	"strings"

	"github.com/Cyclone1070/iav/internal/actionrouter"
	"github.com/Cyclone1070/iav/internal/agent"
	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/eventbus"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/Cyclone1070/iav/internal/tool"
	"github.com/Cyclone1070/iav/internal/tool/bash"
	"github.com/Cyclone1070/iav/internal/tool/edit"
	"github.com/Cyclone1070/iav/internal/tool/glob"
	"github.com/Cyclone1070/iav/internal/tool/grep"
	"github.com/Cyclone1070/iav/internal/tool/question"
	"github.com/Cyclone1070/iav/internal/tool/read"
	"github.com/Cyclone1070/iav/internal/tool/write"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/Cyclone1070/iav/internal/tool/service/checksum"
	"github.com/Cyclone1070/iav/internal/tool/service/path"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/Cyclone1070/iav/internal/ui/prompt"
	"github.com/Cyclone1070/iav/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"golang.org/x/term"
)

const (
	thinkingHeight = 5
	demoTokenLimit = 8192
	chatWidthFixed = 80
	searchRadius   = 24
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("Fatal error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Match normal app behavior unless --debug is used.
	slog.SetDefault(slog.New(slog.DiscardHandler))

	cfg := config.DefaultConfig()
	uiCfg := cfg.UI()

	chatWidth := uiCfg.ChatWindowWidth()
	termHeight := 0
	fd := os.Stdout.Fd()
	if fd <= math.MaxInt {
		if width, height, err := term.GetSize(int(fd)); err == nil && width > 0 {
			if chatWidth <= 0 || width < chatWidth {
				chatWidth = width
			}
			termHeight = height
		}
	}

	pathResolver, err := buildPathResolver()
	if err != nil {
		return fmt.Errorf("path resolver error: %w", err)
	}

	fileSystem := fs.NewOSFileSystem(cfg.Tools().MaxFileSize())
	cmdExecutor := executor.NewOSCommandExecutor(fileSystem)
	checksumMgr := checksum.NewManager(nil, nil)
	taskMgr := bash.NewTaskManager(fileSystem)

	tools := []einotool.BaseTool{
		read.NewTool(fileSystem, checksumMgr, pathResolver),
		edit.NewTool(fileSystem, checksumMgr, pathResolver, cfg.Tools().MaxFileSize()),
		write.NewTool(fileSystem, checksumMgr, pathResolver, cfg.Tools().MaxFileSize()),
		glob.NewTool(fileSystem, cmdExecutor, pathResolver),
		grep.NewTool(fileSystem, cmdExecutor, pathResolver),
		bash.NewTool(fileSystem, cmdExecutor, pathResolver, taskMgr),
		bash.NewSleepTool(taskMgr),
		bash.NewTaskListTool(taskMgr),
		bash.NewTaskStopTool(taskMgr),
		question.NewTool(),
	}
	toolRegistry := tool.NewRegistry(tools)

	bus := eventbus.New()
	defer bus.Close()
	router := actionrouter.New()
	defer router.Close()

	llm := &reproLLM{}
	agentLoop, err := agent.NewGraphRunner(llm, toolRegistry, router, cfg.Tools().MaxIterations(), bus, taskMgr, nil, nil)
	if err != nil {
		return fmt.Errorf("create graph runner: %w", err)
	}

	themeCfg := ui.ThemeConfig{
		PrimaryColor:   ui.ToAdaptiveColor(uiCfg.PrimaryColor()),
		SuccessColor:   ui.ToAdaptiveColor(uiCfg.SuccessColor()),
		ErrorColor:     ui.ToAdaptiveColor(uiCfg.ErrorColor()),
		MutedColor:     ui.ToAdaptiveColor(uiCfg.MutedColor()),
		ShortToolBlock: uiCfg.ShortToolBlock(),
	}
	theme := ui.NewTheme(themeCfg)
	glamour := ui.NewGlamourRenderer(chatWidth, true)
	stream := prompt.NewStream(glamour)
	spinner := ui.NewSpinnerRenderer(lipgloss.NewStyle().Foreground(theme.PrimaryColor()))
	thinking := prompt.NewThinkingRenderer(theme, chatWidth, ui.NewToolOutputGater(thinkingHeight))
	tooling := ui.NewToolRenderer(theme, chatWidth, ui.NewToolOutputGater(uiCfg.BashOutputHeight()))

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

	deps := &workflow.PromptDeps{
		State:        &state.State{},
		Store:        &mockStore{},
		LLM:          llm,
		ToolRegistry: toolRegistry,
		Agent:        agentLoop,
		Bus:          bus,
		Forwarder:    router,
	}

	done := workflow.RunPrompt(context.Background(), "repro list spacing", deps)
	p := tea.NewProgram(uiModel, tea.WithInput(nil), tea.WithOutput(os.Stdout))
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("UI failed: %w", err)
	}
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("workflow failed: %w", err)
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

type reproLLM struct{}

func (m *reproLLM) ID() string          { return "repro-llm" }
func (m *reproLLM) DisplayName() string { return "Repro LLM" }
func (m *reproLLM) ContextWindow() int  { return demoTokenLimit }
func (m *reproLLM) Model() model.ToolCallingChatModel {
	return m
}

func (m *reproLLM) Generate(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return nil, errors.New("not implemented")
}

func (m *reproLLM) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

func (m *reproLLM) Stream(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	// Deterministic single case using the same markdown shape as the reported glitch.
	// Critical split: emit the second bullet marker `*` in its own chunk (without trailing space)
	// to force a transient parse boundary during streaming.
	const markdown = "Ah, I see! You want \"depth\" rather than \"length.\" You want a few items that are rich in detail, rather than a hundred items that are just one word each. \n\nHere is a collection of lists following that \"quality over quantity\" approach.\n\n### 1. The \"Brief\" List (Short items, many of them)\n*This is your classic, rapid-fire list for when you just need the essentials.*\n\n* Milk\n* Eggs\n* Bread\n* Apples\n* Coffee\n* Salt\n* Butter\n\n***\n\n### 2. The \"Deep Dive\" Bulleted List (Few items, long text)\n*This list focuses on a few complex subjects, providing a detailed paragraph for each.*\n\n* **The Aurora Borealis (The Northern Lights):** This breathtaking natural phenomenon occurs when charged particles from the sun collide with gaseous atoms in the Earth's atmosphere. These collisions cause the gases to emit light, creating dancing curtains of green, pink, and violet across the high-latitude skies. The specific colors observed depend largely on which gas is being hit and at what altitude; for instance, oxygen typically produces green or red light, while nitrogen can create blue or purplish hues.\n* **The Concept of Stoicism:** Originating in ancient Greece, Stoicism is a school of Hellenistic philosophy that teaches the development of self-control and fortitude as a means of overcoming destructive emotions. It emphasizes the distinction between what is within our control-our own thoughts, intentions, and actions-and what is not, such as external events, the weather, or the opinions of others. By focusing exclusively on their own character and virtue, Stoics aim to achieve a state of *ataraxia*, or mental tranquility.\n\n***\n\n### 3. The \"Chronological\" Numbered List (Few items, long text)\n*A numbered list used to explain a progression of complex stages.*\n\n1. **The Incubation Phase of Innovation:** Before a new idea ever reaches the public eye, it undergoes a long, often frustrating period of gestation. During this stage, creators encounter countless failures, pivot their designs, and refine their core concepts. It is a time of quiet experimentation where the most significant breakthroughs often happen in isolation, far from the scrutiny of peers or the demands of the market.\n2. **The Scaling and Integration Phase:** Once a concept has proven its viability, the focus shifts from creation to expansion. This involves building the infrastructure necessary to support a larger user base, such as server capacity for software or supply chains for physical goods. It is a delicate balancing act; the organization must grow fast enough to capture momentum but slow enough to ensure that the quality and original vision of the product are not diluted by the sheer weight of its own success.\n3. **The Maturity and Legacy Phase:** As a product or idea becomes a staple of its industry, it enters a state of equilibrium. The rapid growth slows, and the primary goal becomes maintenance and incremental improvement. In this stage, the challenge is to avoid stagnation; successful entities must find ways to innovate within their established framework to stay relevant, eventually building a legacy that influences the next generation of creators."

	chunks := findReproChunks(markdown)

	sr, sw := schema.Pipe[*schema.Message](1)
	go func() {
		defer sw.Close()
		for _, c := range chunks {
			_ = sw.Send(&schema.Message{Role: schema.Assistant, Content: c}, nil)
		}
	}()
	return sr, nil
}

func findReproChunks(markdown string) []string {
	const marker = "\n* **The Concept of Stoicism:"
	markerIdx := strings.Index(markdown, marker)
	if markerIdx < 0 {
		return []string{markdown}
	}

	// Prefer deterministic boundary that *actually* reproduces the gap in streaming.
	start := max(markerIdx-searchRadius, 1)
	const edgeMargin = 2
	end := min(markerIdx+searchRadius, len(markdown)-edgeMargin)
	for i := start; i <= end; i++ {
		chunks := []string{
			markdown[:i],
			markdown[i : i+1],
			markdown[i+1:],
		}
		if reproducesGap(markdown, chunks) {
			return chunks
		}
	}

	// Fallback to the classic forced split.
	i := markerIdx + 1 // emit "*" as a standalone chunk
	return []string{markdown[:i], "*", markdown[i+1:]}
}

func reproducesGap(markdown string, chunks []string) bool {
	renderer := ui.NewGlamourRenderer(chatWidthFixed, true)
	oneShot := stripANSI(renderer.Render(markdown))
	streamed := stripANSI(simulateStream(renderer, chunks))
	gapPattern := regexp.MustCompile(`(?s)Aurora Borealis.*?\n\s*\n\s*• The Concept of Stoicism`)
	return !gapPattern.MatchString(oneShot) && gapPattern.MatchString(streamed)
}

func simulateStream(renderer ui.Renderer, chunks []string) string {
	stream := prompt.NewStream(renderer)
	var out strings.Builder

	for _, ev := range chunks {
		for _, block := range stream.Append(ev) {
			for line := range strings.SplitSeq(block, "\n") {
				out.WriteString(line)
				out.WriteByte('\n')
			}
		}
	}
	for _, block := range stream.Flush() {
		for line := range strings.SplitSeq(block, "\n") {
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}
	return out.String()
}

func stripANSI(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*[mGKH]`)
	return re.ReplaceAllString(s, "")
}

type mockStore struct{}

func (s *mockStore) Create() (*domain.Session, error)       { return &domain.Session{ID: "repro"}, nil }
func (s *mockStore) Get(id string) (*domain.Session, error) { return &domain.Session{ID: id}, nil }
func (s *mockStore) Save(sess *domain.Session) error        { return nil }
func (s *mockStore) GenerateName(ctx context.Context, llm domain.LLM, target string) (string, error) {
	return "Repro Session", nil
}
