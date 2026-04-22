package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Cyclone1070/iav/demo/demoutil"
	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/eventbus"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/Cyclone1070/iav/internal/ui/prompt"
	"github.com/Cyclone1070/iav/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"golang.org/x/term"
)

func main() {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	bus := eventbus.New()
	cfg := config.DefaultConfig().UI()

	chatWidth := cfg.ChatWindowWidth()
	termHeight := 0
	if width, height, err := term.GetSize(int(os.Stdout.Fd())); err == nil && width > 0 {
		if chatWidth <= 0 || width < chatWidth {
			chatWidth = width
		}
		termHeight = height
	}

	themeCfg := ui.ThemeConfig{
		PrimaryColor: ui.ToAdaptiveColor(cfg.PrimaryColor()),
		SuccessColor: ui.ToAdaptiveColor(cfg.SuccessColor()),
		ErrorColor:   ui.ToAdaptiveColor(cfg.ErrorColor()),
		MutedColor:   ui.ToAdaptiveColor(cfg.MutedColor()),
		ShortToolBlock: cfg.ShortToolBlock(),
	}
	theme := ui.NewTheme(themeCfg)
	stream := prompt.NewStream(ui.NewGlamourRenderer(chatWidth, true))
	thinking := prompt.NewThinkingRenderer(theme)
	tooling := ui.NewToolRenderer(theme, chatWidth, ui.NewToolOutputGater(12))
	spinner := ui.NewSpinnerRenderer(lipgloss.NewStyle().Foreground(theme.PrimaryColor()))

	m := prompt.NewModel(
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
		LLM:          &mockLLM{},
		Agent:        &mockAgent{bus: bus},
		Bus:          bus,
		ToolRegistry: &mockRegistry{},
	}

	done := workflow.RunPrompt(context.Background(), "line-based streaming demo", deps)

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running UI: %v\n", err)
		os.Exit(1)
	}

	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		fmt.Printf("Error running workflow: %v\n", err)
		os.Exit(1)
	}
}

type mockAgent struct {
	bus *eventbus.EventBus
}

func (a *mockAgent) Run(ctx context.Context, sess *domain.Session, input string) error {
	tt := demoutil.NewToolTracker(a.bus)
	defer tt.FlushOpenCancelled()

	sleep := func(d time.Duration) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d):
			return nil
		}
	}

	sendChunks := func(chunks []string, gap time.Duration) error {
		for _, c := range chunks {
			a.bus.SendUIUpdate(domain.TextEvent{Text: c})
			if err := sleep(gap); err != nil {
				return err
			}
		}
		return nil
	}

	a.bus.SendUIUpdate(domain.ThinkingEvent{})
	if err := sleep(650 * time.Millisecond); err != nil {
		return err
	}

	intro := []string{
		"Got it",
		" - I'll investigate",
		" the incident across auth logs, deploy history, and webhook retries.",
		" I will stream findings as they land",
		" so the timeline stays readable.\n\n",
		"### In",
		"itial Read\n",
		"I can already see a spike around 09:13 UTC",
		", with most failures clustered in one region.\n",
	}
	if err := sendChunks(intro, 95*time.Millisecond); err != nil {
		return err
	}

	scanDisplay := domain.NewStringDisplay("bash", "rg -n \"authentication failed|token expired\" logs/prod/*.log | head -n 40")
	tt.Start("tool-logs-1", scanDisplay)
	if err := sleep(900 * time.Millisecond); err != nil {
		return err
	}
	tt.End("tool-logs-1", domain.NewStringDisplay("bash",
		"09:13:08 auth middleware: token expired for 731 sessions\n"+
			"09:13:11 payment webhook: signature mismatch for 214 retries\n"+
			"09:13:15 edge gateway: clock skew detected (+93s)\n"))

	if err := sleep(250 * time.Millisecond); err != nil {
		return err
	}

	afterLogs := []string{
		"\nThe first slice confirms two symptoms together: expired auth tokens + webhook signature mismatch.",
		" Common denominator looks like time drift, not malformed payloads.\n\n",
		"Here is the quick triage list as I work:\n",
		"- check",
		" token TTL window\n",
		"- check ",
		"HMAC timestamp tolerance\n",
		"- check ",
		"node clock source and NTP sync\n\n",
		"### Corr",
		"elating Deploys\n",
		"Next I will check infra changes around the same minute.\n",
	}
	if err := sendChunks(afterLogs, 90*time.Millisecond); err != nil {
		return err
	}

	deployDisplay := domain.NewStringDisplay("bash", "git log --oneline --decorate --since=\"3 hours ago\" infra/ deploy/")
	tt.Start("tool-deploy-1", deployDisplay)
	if err := sleep(800 * time.Millisecond); err != nil {
		return err
	}
	tt.End("tool-deploy-1", domain.NewStringDisplay("bash",
		"9ab31f2 deploy(edge): rotate ntp sidecar image to v2.4.0\n"+
			"76cf8d1 infra(time): tighten drift threshold from 120s to 45s\n"+
			"44ed203 ci: update healthcheck timeout\n"))

	if err := sleep(300 * time.Millisecond); err != nil {
		return err
	}

	middleNarrative := []string{
		"\nDeploy history lines up almost exactly with first failures.",
		" That suggests the sidecar image or threshold policy made valid requests fail.\n\n",
		"Proposed detection logic",
		" (pseudo):\n\n",
		"```go\n",
		"func likelyClockSkew(authErr, sigErr int, skewSeconds int) bool {\n",
		"    if authErr == 0 || sigErr == 0 {\n",
		"        return false\n",
		"    }\n",
		"    return skewSeconds > 60\n",
		"}\n",
		"```\n\n",
		"I'll now verify with a direct node-time sample so we avoid guesswork.\n",
	}
	if err := sendChunks(middleNarrative, 85*time.Millisecond); err != nil {
		return err
	}

	timeDisplay := domain.NewStringDisplay("bash", "for n in edge-a edge-b edge-c; do ssh $n 'date -u'; done")
	tt.Start("tool-time-1", timeDisplay)
	if err := sleep(950 * time.Millisecond); err != nil {
		return err
	}
	tt.End("tool-time-1", domain.NewStringDisplay("bash",
		"edge-a Tue Apr 21 09:14:41 UTC 2026\n"+
			"edge-b Tue Apr 21 09:13:08 UTC 2026\n"+
			"edge-c Tue Apr 21 09:14:42 UTC 2026\n"))

	longWrapSection := []string{
		"\nThat confirms it: edge-b is behind by ~93 seconds.",
		" That is enough to break token validation windows and HMAC timestamp checks in strict mode.\n\n",
		"Current state table:\n\n",
		"| Node | Drift (s) | Auth Failures/min | Webhook Signature Fails/min |\n",
		"| --- | ---: | ---: | ---: |\n",
		"| edge-a | 1 | 3 | 1 |\n",
		"| edge-b | 93 | 731 | 214 |\n",
		"| edge-c | 0 | 4 | 0 |\n\n",
		"### Recommended Mitigation\n",
		"1",
		". Roll back NTP sidecar on edge-b immediately.\n",
		"2. Temporarily relax skew tolerance to 120s",
		" for 15 minutes.\n",
		"3. Replay failed webhooks from dead-letter queue after clock sync.\n",
		"4. Add drift alerting so this never goes silent again.\n",
	}
	if err := sendChunks(longWrapSection, 95*time.Millisecond); err != nil {
		return err
	}

	rollbackDisplay := domain.NewStringDisplay("bash", "kubectl rollout undo ds/ntp-sidecar -n edge --to-revision=41")
	tt.Start("tool-rollback-1", rollbackDisplay)
	if err := sleep(850 * time.Millisecond); err != nil {
		return err
	}
	tt.End("tool-rollback-1", domain.NewStringDisplay("bash",
		"daemonset.apps/ntp-sidecar rolled back\n"+
			"waiting for nodes: edge-b updated, edge-a unchanged, edge-c unchanged\n"+
			"rollout complete\n"))

	final := []string{
		"\nMitigation has been applied and rollout is complete. ",
		"Error rate should start dropping within one to two token refresh windows.\n\n",
		"### Final Summary\n",
		"- Root cause",
		": edge-b clock drift after NTP sidecar upgrade.\n",
		"- Impact: token expiry false-positives + webhook signature mismatch.\n",
		"- Status: rollback completed; monitor next 10 minutes and replay dead-letter queue.\n\n",
		"Follow-up command draft:\n\n",
		"```bash\n",
		"kubectl logs -n edge ds/ntp-sidecar --since=15m | rg -n \"drift|sync|offset\"\n",
		"```\n",
	}
	if err := sendChunks(final, 90*time.Millisecond); err != nil {
		return err
	}

	return nil
}

type mockStore struct{}

func (s *mockStore) Create() (*domain.Session, error)       { return &domain.Session{ID: "line-demo"}, nil }
func (s *mockStore) Get(id string) (*domain.Session, error) { return &domain.Session{ID: id}, nil }
func (s *mockStore) Save(sess *domain.Session) error        { return nil }
func (s *mockStore) GenerateName(ctx context.Context, llm domain.LLM, target string) (string, error) {
	return "Line Streaming Demo", nil
}

type mockLLM struct{}

func (l *mockLLM) ID() string                        { return "mock" }
func (l *mockLLM) DisplayName() string               { return "Mock LLM" }
func (l *mockLLM) ContextWindow() int                { return 1000 }
func (l *mockLLM) Model() model.ToolCallingChatModel { return nil }
func (l *mockLLM) ComputeTokens(ctx context.Context, msgs []*schema.Message) (int, error) {
	return 0, nil
}

type mockRegistry struct{}

func (r *mockRegistry) Definitions() []*schema.ToolInfo     { return nil }
func (r *mockRegistry) Get(name string) (domain.Tool, bool) { return nil, false }
