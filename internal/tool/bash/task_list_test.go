package bash

import (
	"context"
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/stretchr/testify/assert"
)

func TestTaskListTool_Execute(t *testing.T) {
	tm := NewTaskManager(nil)
	tl := NewTaskListTool(tm)
	ctx := context.Background()

	// Register a dummy task
	cmd := executor.NewStreamingCmd("t-list-1", strings.NewReader(""), func() (*executor.Result, error) {
		return &executor.Result{ExitCode: 0}, nil
	}, "")
	_ = tm.Register("t-list-1", cmd, "", func() {}, "list files", "ls -R")

	inv, err := tl.Prepare("{}")
	assert.NoError(t, err)

	llm, disp := inv.(domain.ExecutableInvocation).Execute(ctx)
	
	assert.Contains(t, llm, "active background bash tasks:") // lowercase
	assert.Contains(t, llm, "t-list-1"); assert.Contains(t, llm, "list files"); assert.Contains(t, llm, "ls -R")
	assert.Contains(t, llm, "Status: running")
	assert.Equal(t, "List active background bash tasks", disp.(domain.StringDisplay).Description)
}

func TestTaskListTool_Execute_Cancelled(t *testing.T) {
	tm := NewTaskManager(nil)
	tl := NewTaskListTool(tm)
	inv, _ := tl.Prepare("{}")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, display := inv.(domain.ExecutableInvocation).Execute(ctx)
	// Red Phase: This currently returns "execution cancelled" as summary
	assert.Equal(t, "List active background bash tasks", display.(domain.StringDisplay).Description)
}
