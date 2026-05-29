// Package bash provides tools for executing shell commands.
package bash

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Cyclone1070/autocmd/internal/domain"
	"github.com/Cyclone1070/autocmd/internal/runtimectx"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const taskStopToolName = "task_stop"

type taskStopper interface {
	Stop(id string) error
}

// TaskStopTool is a tool for terminating background tasks.
type TaskStopTool struct {
	manager taskStopper
}

// NewTaskStopTool creates a new TaskStopTool.
func NewTaskStopTool(manager taskStopper) *TaskStopTool {
	return &TaskStopTool{
		manager: manager,
	}
}

// IsConcurrentSafe indicates if the task stop tool can be run concurrently.
func (t *TaskStopTool) IsConcurrentSafe() bool { return true }

func (t *TaskStopTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: taskStopToolName,
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
	}, nil
}

func (t *TaskStopTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	req, err := t.validate(argumentsInJSON)
	if err != nil {
		return "", err
	}
	callID := compose.GetToolCallID(ctx)
	llmContent, finalDisplay := t.executeTaskStop(ctx, req)
	if events, ok := runtimectx.EventSenderFrom(ctx); ok && events != nil {
		events.SendUIUpdate(domain.ToolEndEvent{CallID: callID, Display: finalDisplay})
	}
	if sink, ok := runtimectx.ToolDisplaySinkFrom(ctx); ok && sink != nil {
		sink(callID, finalDisplay)
	}
	return llmContent, nil
}

func (t *TaskStopTool) Preview(input *compose.ToolInput) domain.ToolDisplay {
	req := &taskStopRequest{}
	if err := json.Unmarshal([]byte(input.Arguments), req); err != nil {
		return domain.NewStringDisplay(fmt.Sprintf("Run \"%s\"", taskStopToolName), "")
	}
	return domain.NewStringDisplay(fmt.Sprintf("Stop background bash task %s", req.TaskID), "")
}

func (t *TaskStopTool) PreflightValidate(input *compose.ToolInput) error {
	_, err := t.validate(input.Arguments)
	return err
}

type taskStopRequest struct {
	TaskID string `json:"task_id"`
}

type validatedTaskStopRequest struct {
	taskID string
}

func (t *TaskStopTool) validate(params string) (*validatedTaskStopRequest, error) {
	var req taskStopRequest
	if err := json.Unmarshal([]byte(params), &req); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	if req.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}

	return &validatedTaskStopRequest{
		taskID: req.TaskID,
	}, nil
}

func (t *TaskStopTool) executeTaskStop(ctx context.Context, req *validatedTaskStopRequest) (string, domain.ToolDisplay) {
	display := domain.NewStringDisplay(fmt.Sprintf("Stop background bash task %s", req.taskID), "")
	if ctx.Err() != nil {
		display.Error = domain.ToolErrorCancelled
		return domain.ToolErrorCancelled, display
	}

	err := t.manager.Stop(req.taskID)
	if err != nil {
		llm := fmt.Sprintf("error: %v", err)
		display.Error = domain.ToolErrorFailed
		return llm, display
	}

	llm := fmt.Sprintf("task %s stopped", req.taskID)
	return llm, display
}
