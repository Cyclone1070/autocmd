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
	HasPending() bool
}

// SleepTool provides a way for the LLM to wait for a specific duration or for background tasks to complete.
type SleepTool struct {
	notifier completionNotifier
}

// NewSleepTool creates a new SleepTool with the provided notifier.
func NewSleepTool(notifier completionNotifier) *SleepTool {
	return &SleepTool{
		notifier: notifier,
	}
}

// Name returns the name of the tool.
func (t *SleepTool) Name() string {
	return "sleep"
}

// IsConcurrentSafe returns true as sleep is safe to run concurrently.
func (t *SleepTool) IsConcurrentSafe() bool { return true }

// Definition returns the JSON schema for the sleep tool.
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

// Prepare validates the sleep request and returns an invocation.
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
		display:    domain.NewStringDisplay(fmt.Sprintf("Sleep for %s", duration.String()), ""),
	}, nil
}

type sleepInvocation struct {
	notifier   completionNotifier
	display    domain.StringDisplay
	durationMS int
}

func (i *sleepInvocation) Display() domain.ToolDisplay {
	return i.display
}

func (i *sleepInvocation) Execute(ctx context.Context) (string, domain.ToolDisplay) {
	start := time.Now()

	if i.notifier.HasPending() {
		elapsed := time.Since(start).Round(time.Second).String()
		llm := fmt.Sprintf("sleep interrupted after %s: background bash process finished", elapsed)
		i.display.Description = fmt.Sprintf("Sleep interrupted after %s: background bash process finished", elapsed)
		i.display.Content = ""
		return llm, i.display
	}

	duration := time.Duration(i.durationMS) * time.Millisecond

	select {
	case <-time.After(duration):
		llm := fmt.Sprintf("slept for %dms", i.durationMS)
		return llm, i.display
	case <-i.notifier.NotifyChan():
		elapsed := time.Since(start).Round(time.Second).String()
		llm := fmt.Sprintf("sleep interrupted after %s: background bash process finished", elapsed)
		i.display.Description = fmt.Sprintf("Sleep interrupted after %s: background bash process finished", elapsed)
		i.display.Content = ""
		return llm, i.display
	case <-ctx.Done():
		i.display.Error = domain.ToolErrorCancelled
		return domain.ToolErrorCancelled, i.display
	}
}
