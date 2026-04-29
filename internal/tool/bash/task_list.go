// Package bash provides tools for executing shell commands.
package bash

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
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

// Definition returns the Eino tool definition for the task list tool.
func (t *TaskListTool) Definition() *schema.ToolInfo {
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
	}
}

// Prepare returns an invocation for the task list tool.
func (t *TaskListTool) Prepare(_ string) (domain.Invocation, error) {
	return &taskListInvocation{
		manager: t.manager,
		display: domain.NewStringDisplay("List active background bash tasks", ""),
	}, nil
}

type taskListInvocation struct {
	manager taskLister
	display domain.StringDisplay
}

func (i *taskListInvocation) Display() domain.ToolDisplay {
	return i.display
}

func (i *taskListInvocation) Execute(ctx context.Context) (string, domain.ToolDisplay) {
	if ctx.Err() != nil {
		i.display.Error = domain.ToolErrorCancelled
		return domain.ToolErrorCancelled, i.display
	}

	tasks := i.manager.List()
	if len(tasks) == 0 {
		return "no active background tasks", i.display
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

	return sbLLM.String(), i.display
}
