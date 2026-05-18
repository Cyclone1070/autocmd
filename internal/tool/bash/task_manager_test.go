package bash

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/stretchr/testify/assert"
)

func TestTaskManager_RegisterAndDrain(t *testing.T) {
	tm := NewTaskManager(nil)

	// Mock a process that finishes immediately
	waitCalled := make(chan struct{})
	cmd := executor.NewStreamingCmd("t1", strings.NewReader("output"), func() (*executor.Result, error) {
		close(waitCalled)
		return &executor.Result{ExitCode: 0, Stdout: "final output"}, nil
	}, "")

	err := tm.Register("t1", cmd, "/tmp/bash_t1.log", func() {}, "test description", "test command", "/test/cwd")
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	// Wait for task to complete internally
	select {
	case <-waitCalled:
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for task to complete")
	}

	// Give a small grace period for the goroutine to update the queue
	time.Sleep(50 * time.Millisecond)

	notifications := tm.Drain()
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}

	notif := notifications[0]
	assert.Equal(t, "t1", notif.ID)
	assert.Equal(t, "execution completed", notif.Status)
	assert.Equal(t, "test description", notif.Description)
	assert.Equal(t, "test command", notif.Command)
	assert.Equal(t, "/test/cwd", notif.Cwd)
}

func TestTaskManager_ActivityTracking(t *testing.T) {
	tm := NewTaskManager(nil)

	// Create a command that won't finish immediately
	cmd := executor.NewStreamingCmd("t1", strings.NewReader(""), func() (*executor.Result, error) {
		time.Sleep(100 * time.Millisecond)
		return &executor.Result{ExitCode: 0}, nil
	}, "")

	// Manual update to activity
	cmd.UpdateActivity()

	_ = tm.Register("t1", cmd, "/tmp/t1.log", func() {}, "test", "command", "/tmp")

	tasks := tm.List()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}

	// Since we just updated it, SecondsSinceActivity should be 0 or 1
	if tasks[0].SecondsSinceActivity < 0 || tasks[0].SecondsSinceActivity > 2 {
		t.Errorf("Unexpected SecondsSinceActivity: %d", tasks[0].SecondsSinceActivity)
	}
}

func TestTaskManager_NotifyChan(t *testing.T) {
	tm := NewTaskManager(nil)

	notify := tm.NotifyChan()

	cmd := executor.NewStreamingCmd("t1", strings.NewReader(""), func() (*executor.Result, error) {
		return &executor.Result{ExitCode: 0}, nil
	}, "")

	_ = tm.Register("t1", cmd, "/tmp/t1.log", func() {}, "test description", "test command", "/tmp")

	select {
	case <-notify:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("NotifyChan did not signal completion")
	}
}



func TestTaskManager_HandleCompletion_Constants(t *testing.T) {
	tm := NewTaskManager(nil)

	t.Run("zero exit code -> execution completed", func(t *testing.T) {
		tm.handleCompletion("t1", &executor.Result{ExitCode: 0}, nil, "/tmp/t1.log")
		notif := tm.Drain()[0]
		assert.Equal(t, "execution completed", notif.Status)
		assert.Equal(t, 0, notif.ExitCode)
	})

	t.Run("non-zero exit code -> still execution completed", func(t *testing.T) {
		tm.handleCompletion("t2", &executor.Result{ExitCode: 1}, nil, "/tmp/t2.log")
		notif := tm.Drain()[0]
		assert.Equal(t, "execution completed", notif.Status)
		assert.Equal(t, 1, notif.ExitCode)
	})

	t.Run("context cancelled -> ToolErrorCancelled", func(t *testing.T) {
		tm.handleCompletion("t3", &executor.Result{ExitCode: -1}, context.Canceled, "/tmp/t3.log")
		notif := tm.Drain()[0]
		assert.Equal(t, string(domain.ToolErrorCancelled), notif.Status)
	})

	t.Run("context deadline exceeded -> ToolErrorTimedOut as status", func(t *testing.T) {
		tm.handleCompletion("t4", &executor.Result{ExitCode: -1}, context.DeadlineExceeded, "/tmp/t4.log")
		notif := tm.Drain()[0]
		assert.Equal(t, string(domain.ToolErrorTimedOut), notif.Status)
	})

	t.Run("system error -> ToolErrorFailed with error field", func(t *testing.T) {
		tm.handleCompletion("t5", &executor.Result{ExitCode: -1}, fmt.Errorf("system crash"), "/tmp/t5.log")
		notif := tm.Drain()[0]
		assert.Equal(t, string(domain.ToolErrorFailed), notif.Status)
		assert.Equal(t, "system crash", notif.Error)
	})
}
