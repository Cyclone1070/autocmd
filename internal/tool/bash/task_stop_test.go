package bash

import (
	"context"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestTaskStopTool_Execute_Success(t *testing.T) {
	ctx := context.Background()

	// I'll use a mock TaskManager for this specific test case to simplify.
	mockTM := new(mockTaskManager)
	tl := NewTaskStopTool(mockTM)

	mockTM.On("Stop", "t-stop-1").Return(nil)

	params := `{"task_id": "t-stop-1"}`
	inv, err := tl.Prepare(params)
	assert.NoError(t, err)
	resLLM, disp := inv.(domain.ExecutableInvocation).Execute(ctx)

	assert.Equal(t, "task t-stop-1 stopped", resLLM)
	assert.Equal(t, "Stop background bash task t-stop-1", disp.(domain.StringDisplay).Description)
	mockTM.AssertExpectations(t)
}

func TestTaskStopTool_Execute_NotFound(t *testing.T) {
	tm := NewTaskManager(nil)
	tl := NewTaskStopTool(tm)
	ctx := context.Background()

	params := `{"task_id": "none"}`
	inv, _ := tl.Prepare(params)
	_, disp := inv.(domain.ExecutableInvocation).Execute(ctx)

	assert.Equal(t, domain.ToolErrorFailed, disp.GetError())
}

func TestTaskStopTool_Execute_Cancelled(t *testing.T) {
	tm := NewTaskManager(nil)
	tl := NewTaskStopTool(tm)
	inv, _ := tl.Prepare("{\"task_id\": \"t1\"}")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, display := inv.(domain.ExecutableInvocation).Execute(ctx)
	// Red Phase: This currently returns "execution cancelled" as summary
	assert.Equal(t, "Stop background bash task t1", display.(domain.StringDisplay).Description)
}
