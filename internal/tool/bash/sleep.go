package bash

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/runtimectx"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const sleepToolName = "sleep"

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


// IsConcurrentSafe returns true as sleep is safe to run concurrently.
func (t *SleepTool) IsConcurrentSafe() bool { return true }

func (t *SleepTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: sleepToolName,
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
	}, nil
}

func (t *SleepTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	req, err := t.validate(argumentsInJSON)
	if err != nil {
		return "", err
	}
	callID := compose.GetToolCallID(ctx)
	llmContent, finalDisplay := t.executeSleep(ctx, req)
	if events, ok := runtimectx.EventSenderFrom(ctx); ok && events != nil {
		events.SendUIUpdate(domain.ToolEndEvent{CallID: callID, Display: finalDisplay})
	}
	if sink, ok := runtimectx.ToolDisplaySinkFrom(ctx); ok && sink != nil {
		sink(callID, finalDisplay)
	}
	return llmContent, nil
}

func (t *SleepTool) Preview(input *compose.ToolInput) domain.ToolDisplay {
	var req sleepRequest
	if err := json.Unmarshal([]byte(input.Arguments), &req); err != nil {
		return domain.NewStringDisplay(fmt.Sprintf("Run \"%s\"", sleepToolName), "")
	}
	duration := time.Duration(req.DurationMS) * time.Millisecond
	return domain.NewStringDisplay(fmt.Sprintf("Sleep for %s", duration.String()), "")
}

func (t *SleepTool) PreflightValidate(input *compose.ToolInput) error {
	_, err := t.validate(input.Arguments)
	return err
}

type sleepRequest struct {
	DurationMS int `json:"duration_ms"`
}

type validatedSleepRequest struct {
	durationMS int
}

func (t *SleepTool) validate(params string) (*validatedSleepRequest, error) {
	var req sleepRequest
	if err := json.Unmarshal([]byte(params), &req); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	if req.DurationMS <= 0 {
		return nil, fmt.Errorf("duration_ms must be positive")
	}

	return &validatedSleepRequest{
		durationMS: req.DurationMS,
	}, nil
}

func (t *SleepTool) executeSleep(ctx context.Context, req *validatedSleepRequest) (string, domain.ToolDisplay) {
	start := time.Now()
	duration := time.Duration(req.durationMS) * time.Millisecond
	display := domain.NewStringDisplay(fmt.Sprintf("Sleep for %s", duration.String()), "")

	if t.notifier.HasPending() {
		elapsed := time.Since(start).Round(time.Second).String()
		llm := fmt.Sprintf("sleep interrupted after %s: background bash process finished", elapsed)
		display.Description = fmt.Sprintf("Sleep interrupted after %s: background bash process finished", elapsed)
		display.Content = ""
		return llm, display
	}

	select {
	case <-time.After(duration):
		llm := fmt.Sprintf("slept for %dms", req.durationMS)
		return llm, display
	case <-t.notifier.NotifyChan():
		elapsed := time.Since(start).Round(time.Second).String()
		llm := fmt.Sprintf("sleep interrupted after %s: background bash process finished", elapsed)
		display.Description = fmt.Sprintf("Sleep interrupted after %s: background bash process finished", elapsed)
		display.Content = ""
		return llm, display
	case <-ctx.Done():
		display.Error = domain.ToolErrorCancelled
		return domain.ToolErrorCancelled, display
	}
}
