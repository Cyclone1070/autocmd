package bash

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
)

type taskStopper interface {
	Stop(id string) error
}

type TaskStopTool struct {
	manager taskStopper
}

func NewTaskStopTool(manager taskStopper) *TaskStopTool {
	return &TaskStopTool{
		manager: manager,
	}
}

func (t *TaskStopTool) Name() string {
	return "task_stop"
}

func (t *TaskStopTool) IsConcurrentSafe() bool { return true }

func (t *TaskStopTool) Definition() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: "task_stop",
		Desc: `Terminates a running background task by its ID.

Usage:
- Stops a running background task by its ID.
- Returns a success or failure status.
- Use this tool when you need to terminate a long-running task that is no longer needed or is consuming too many resources.`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"task_id": {
				Type:     schema.String,
				Desc:     "Task identifier identifying the task to stop.",
				Required: true,
			},
		}),
	}
}

func (t *TaskStopTool) Prepare(params string) (domain.Invocation, error) {
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal([]byte(params), &req); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	if req.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}

	return &taskStopInvocation{
		manager: t.manager,
		taskID:  req.TaskID,
		display: domain.NewStringDisplay(fmt.Sprintf("Stop background bash task %s", req.TaskID), ""),
	}, nil
}

type taskStopInvocation struct {
	manager taskStopper
	taskID  string
	display domain.StringDisplay
}

func (i *taskStopInvocation) Display() domain.ToolDisplay {
	return i.display
}

func (i *taskStopInvocation) Execute(ctx context.Context) (string, domain.ToolDisplay) {
	if ctx.Err() != nil {
		i.display.Error = domain.ToolErrorCancelled
		return domain.ToolErrorCancelled, i.display
	}

	err := i.manager.Stop(i.taskID)
	if err != nil {
		llm := fmt.Sprintf("error: %v", err)
		i.display.Error = domain.ToolErrorFailed
		return llm, i.display
	}

	llm := fmt.Sprintf("task %s stopped", i.taskID)
	return llm, i.display
}
