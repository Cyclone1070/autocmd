package bash

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
)

type completionNotifier interface {
	NotifyChan() <-chan struct{}
}

type SleepTool struct {
	notifier completionNotifier
}

func NewSleepTool(notifier completionNotifier) *SleepTool {
	return &SleepTool{
		notifier: notifier,
	}
}

func (t *SleepTool) Name() string {
	return "sleep"
}

func (t *SleepTool) IsConcurrentSafe() bool { return true }

func (t *SleepTool) Definition() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: "sleep",
		Desc: `Wait for a specified duration or until a background bash task completes.

Usage:
- Use this when you have nothing to do, or when you're waiting for something.
- Use this tool before finishing your turn if you have background tasks (like builds or tests) that must finish before you stop talking.
- You can call this concurrently with other tools — it won't interfere with them.
- Prefer this over "Bash(sleep ...)" — it doesn't hold a shell process.
- Do not sleep between commands that can run immediately — just run them.
- If your command is long running and you would like to be notified when it finishes — use "bash" with "run_in_background" instead. No sleep needed.
- Do not retry failing commands in a sleep loop — diagnose the root cause.
- If you must poll an external process, use a check command (e.g. "git status" or "ls") rather than sleeping first.`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"duration_ms": {
				Type:     schema.Integer,
				Desc:     "The duration to sleep in milliseconds.",
				Required: true,
			},
		}),
	}
}

func (t *SleepTool) Prepare(params string) (domain.Invocation, error) {
	var req struct {
		DurationMS int `json:"duration_ms"`
	}
	if err := json.Unmarshal([]byte(params), &req); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	if req.DurationMS <= 0 {
		return nil, fmt.Errorf("duration_ms must be positive")
	}

	duration := time.Duration(req.DurationMS) * time.Millisecond
	return &sleepInvocation{
		notifier:   t.notifier,
		durationMS: req.DurationMS,
		display:    domain.NewStringDisplay("", fmt.Sprintf("SLEEP for %s", duration.String())),
	}, nil
}

type sleepInvocation struct {
	notifier   completionNotifier
	durationMS int
	display    domain.StringDisplay
}

func (i *sleepInvocation) Display() domain.ToolDisplay {
	return i.display
}

func (i *sleepInvocation) Execute(ctx context.Context) (string, domain.ToolDisplay) {
	duration := time.Duration(i.durationMS) * time.Millisecond

	select {
	case <-time.After(duration):
		llm := fmt.Sprintf("slept for %dms", i.durationMS)
		return llm, i.display
	case <-i.notifier.NotifyChan():
		llm := "sleep interrupted: background bash process finished"
		i.display.Content = "SLEEP interrupted: background bash process finished"
		return llm, i.display
	case <-ctx.Done():
		i.display.Error = domain.ToolErrorCancelled
		return domain.ToolErrorCancelled, i.display
	}
}
