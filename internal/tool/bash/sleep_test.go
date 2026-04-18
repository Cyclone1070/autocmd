package bash

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/stretchr/testify/assert"
)

func TestSleepTool_Execute_Success(t *testing.T) {
	tm := NewTaskManager(nil)
	tl := NewSleepTool(tm)
	ctx := context.Background()

	params := `{"duration_ms": 10}`
	inv, _ := tl.Prepare(params)

	llm, disp := inv.(domain.ExecutableInvocation).Execute(ctx)
	assert.Equal(t, "slept for 10ms", llm)
	assert.Equal(t, "SLEEP for 10ms", disp.(domain.StringDisplay).Content)
}

func TestSleepTool_Execute_Interrupted(t *testing.T) {
	tm := NewTaskManager(nil)
	tl := NewSleepTool(tm)
	ctx := context.Background()

	cmd := executor.NewStreamingCmd("t1", strings.NewReader(""), func() (*executor.Result, error) {
		time.Sleep(50 * time.Millisecond)
		return &executor.Result{ExitCode: 0}, nil
	}, "")
	_ = tm.Register("t1", cmd, "", func() {}, "desc", "cmd")

	params := `{"duration_ms": 5000}`
	inv, _ := tl.Prepare(params)

	llm, disp := inv.(domain.ExecutableInvocation).Execute(ctx)
	assert.Equal(t, "sleep interrupted: background bash process finished", llm)
	assert.Equal(t, "SLEEP interrupted: background bash process finished", disp.(domain.StringDisplay).Content)
}

func TestSleepTool_Execute_Cancelled(t *testing.T) {
	tm := NewTaskManager(nil)
	tl := NewSleepTool(tm)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	params := `{"duration_ms": 1000}`
	inv, _ := tl.Prepare(params)
	_, disp := inv.(domain.ExecutableInvocation).Execute(ctx)
	
	assert.Equal(t, domain.ToolErrorCancelled, disp.(domain.StringDisplay).Error)
	assert.Equal(t, "SLEEP for 1s", disp.(domain.StringDisplay).Content)
}

func TestSleepTool_Execute_AlreadyFinished(t *testing.T) {
	tm := NewTaskManager(nil)
	tl := NewSleepTool(tm)
	ctx := context.Background()

	// 1. Start a task that finishes immediately
	cmd := executor.NewStreamingCmd("t1", strings.NewReader(""), func() (*executor.Result, error) {
		return &executor.Result{ExitCode: 0}, nil
	}, "")
	_ = tm.Register("t1", cmd, "log1", func() {}, "desc", "cmd")

	// 2. Wait for it to finish and be processed by TaskManager
	// The goroutine in Register calls handleCompletion which closes notifyChan and replaces it.
	time.Sleep(100 * time.Millisecond)

	// 3. Call sleep. It should return immediately because a task is already done.
	params := `{"duration_ms": 1000}`
	inv, _ := tl.Prepare(params)

	start := time.Now()
	llm, disp := inv.(domain.ExecutableInvocation).Execute(ctx)
	duration := time.Since(start)

	assert.Less(t, duration, 500*time.Millisecond, "Sleep should have been interrupted immediately, but took %v", duration)
	assert.Equal(t, "sleep interrupted: background bash process finished", llm)
	assert.Equal(t, "SLEEP interrupted: background bash process finished", disp.(domain.StringDisplay).Content)
}
