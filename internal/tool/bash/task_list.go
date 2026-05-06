// Package bash provides tools for executing shell commands.
package bash

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/runtimectx"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const stalledThresholdSeconds = 30

type taskLister interface {
	List() []TaskInfo
}

// TaskListTool is a tool for listing active background tasks.
type TaskListTool struct {
	manager taskLister
}

// NewTaskListTool creates a new TaskListTool.
func NewTaskListTool(manager taskLister) *TaskListTool {
	return &TaskListTool{
		manager: manager,
	}
}

// Name returns the unique identifier for the task list tool.
func (t *TaskListTool) Name() string {
	return "task_list"
}

// IsConcurrentSafe indicates if the task list tool can be run concurrently.
func (t *TaskListTool) IsConcurrentSafe() bool { return true }

func (t *TaskListTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "task_list",
		Desc: `List all active background bash tasks.

## When to Use This Tool
- To see what background tasks are currently running.
- To check overall progress on long-running commands.
- To find task IDs for use with the task_stop tool.

## Output
Returns a summary of each task:
- **ID**: Task identifier (use with task_stop).
- **Description**: Brief description of the task provided when it was started.
- **Command**: The actual bash command being executed.
- **Status**: Whether the task is still running or potentially stalled.`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{}),
	}, nil
}

func (t *TaskListTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	callID := compose.GetToolCallID(ctx)
	llmContent, finalDisplay := t.executeTaskList(ctx)
	if events, ok := runtimectx.EventSenderFrom(ctx); ok && events != nil {
		events.SendUIUpdate(domain.ToolEndEvent{CallID: callID, Display: finalDisplay})
	}
	if sink, ok := runtimectx.ToolDisplaySinkFrom(ctx); ok && sink != nil {
		sink(callID, finalDisplay)
	}
	return llmContent, nil
}

func (t *TaskListTool) Preview(input *compose.ToolInput) domain.ToolDisplay {
	return domain.NewStringDisplay("List active background bash tasks", "")
}

func (t *TaskListTool) PreflightValidate(input *compose.ToolInput) error {
	return nil
}

func (t *TaskListTool) executeTaskList(ctx context.Context) (string, domain.ToolDisplay) {
	display := domain.NewStringDisplay("List active background bash tasks", "")
	if ctx.Err() != nil {
		display.Error = domain.ToolErrorCancelled
		return domain.ToolErrorCancelled, display
	}

	tasks := t.manager.List()
	if len(tasks) == 0 {
		return "no active background tasks", display
	}

	var sbLLM strings.Builder
	sbLLM.WriteString("active background bash tasks:\n")
	for _, t := range tasks {
		status := "running"
		if t.SecondsSinceActivity > stalledThresholdSeconds {
			status = "POTENTIALLY STALLED"
		}

		activityStr := "just now"
		if t.SecondsSinceActivity > 0 {
			activityStr = (time.Duration(t.SecondsSinceActivity) * time.Second).String()
		}

		fmt.Fprintf(&sbLLM, "- ID: %s\n  Description: %s\n  Command: %s\n  Status: %s (last activity: %s)\n",
			t.ID, t.Description, t.Command, status, activityStr)
	}

	return sbLLM.String(), display
}
