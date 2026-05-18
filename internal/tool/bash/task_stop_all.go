// Package bash provides tools for executing shell commands.
package bash

import (
	"context"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/runtimectx"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type taskStopAller interface {
	StopAll()
}

// TaskStopAllTool is a tool for terminating all background tasks.
type TaskStopAllTool struct {
	manager taskStopAller
}

// NewTaskStopAllTool creates a new TaskStopAllTool.
func NewTaskStopAllTool(manager taskStopAller) *TaskStopAllTool {
	return &TaskStopAllTool{
		manager: manager,
	}
}

// Name returns the unique identifier for the task stop all tool.
func (t *TaskStopAllTool) Name() string {
	return "task_stop_all"
}

// IsConcurrentSafe indicates if the tool can be run concurrently.
func (t *TaskStopAllTool) IsConcurrentSafe() bool { return true }

func (t *TaskStopAllTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "task_stop_all",
		Desc: `Terminates all active background tasks immediately. Use this tool when you decide you no longer need running tasks, or when you are ready to finish your response and need to clean up active tasks.`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{}),
	}, nil
}

func (t *TaskStopAllTool) InvokableRun(ctx context.Context, _ string, _ ...einotool.Option) (string, error) {
	callID := compose.GetToolCallID(ctx)
	llmContent, finalDisplay := t.executeTaskStopAll(ctx, nil)
	if events, ok := runtimectx.EventSenderFrom(ctx); ok && events != nil {
		events.SendUIUpdate(domain.ToolEndEvent{CallID: callID, Display: finalDisplay})
	}
	if sink, ok := runtimectx.ToolDisplaySinkFrom(ctx); ok && sink != nil {
		sink(callID, finalDisplay)
	}
	return llmContent, nil
}

func (t *TaskStopAllTool) Preview(input *compose.ToolInput) domain.ToolDisplay {
	return domain.NewStringDisplay("Stop all background bash tasks", "")
}

func (t *TaskStopAllTool) PreflightValidate(input *compose.ToolInput) error {
	return nil
}

// validate is not needed as there are no parameters

func (t *TaskStopAllTool) executeTaskStopAll(ctx context.Context, _ any) (string, domain.ToolDisplay) {
	display := domain.NewStringDisplay("Stop all background bash tasks", "")
	if ctx.Err() != nil {
		display.Error = domain.ToolErrorCancelled
		return domain.ToolErrorCancelled, display
	}

	t.manager.StopAll()

	llm := "all background tasks stopped"
	return llm, display
}
