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
		Desc: "Wait for a specified duration or until a background bash task completes.",
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
