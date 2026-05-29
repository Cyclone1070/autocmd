package bash

import (
	"context"
	"strings"
	"testing"

	"github.com/Cyclone1070/autocmd/internal/domain"
	"github.com/Cyclone1070/autocmd/internal/tool/service/executor"
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
	_ = tm.Register("t-list-1", cmd, "", func() {}, "list files", "ls -R", "/tmp")

	llm, disp := tl.executeTaskList(ctx)

	assert.Contains(t, llm, "active background bash tasks:") // lowercase
	assert.Contains(t, llm, "t-list-1")
	assert.Contains(t, llm, "list files")
	assert.Contains(t, llm, "ls -R")
	assert.Contains(t, llm, "Status: running")
	assert.Equal(t, "List active background bash tasks", disp.(domain.StringDisplay).Description)
}

func TestTaskListTool_Execute_Cancelled(t *testing.T) {
	tm := NewTaskManager(nil)
	tl := NewTaskListTool(tm)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, display := tl.executeTaskList(ctx)
	// Red Phase: This currently returns "execution cancelled" as summary
	assert.Equal(t, "List active background bash tasks", display.(domain.StringDisplay).Description)
}
