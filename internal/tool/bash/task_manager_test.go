package bash

import (
	"context"
	"fmt"
	"io"
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

	err := tm.Register("t1", cmd, "/tmp/bash_t1.log", func() {}, "test description", "test command")
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
	assert.Contains(t, notif, "<task-id>t1</task-id>")
	assert.Contains(t, notif, "<status>execution completed</status>")
	assert.Contains(t, notif, "<description>test description</description>")
	assert.Contains(t, notif, "<command>test command</command>")
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

	_ = tm.Register("t1", cmd, "/tmp/t1.log", func() {}, "test", "command")

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

	_ = tm.Register("t1", cmd, "/tmp/t1.log", func() {}, "test description", "test command")

	select {
	case <-notify:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("NotifyChan did not signal completion")
	}
}

func TestTaskManager_Register_ReconstructsLogPath(t *testing.T) {
	fs := &mockBashFileSystem{files: make(map[string][]byte)}
	tm := NewTaskManager(fs)

	logPath := "/tmp/task.log"
	cmd := executor.NewStreamingCmd("t1", strings.NewReader(""), func() (*executor.Result, error) {
		// Mock result with Stdout but no LogPath (simulating <16kb deletion)
		return &executor.Result{ExitCode: 0, Stdout: "secret output", LogPath: ""}, nil
	}, logPath)

	cancel := func() {}
	err := tm.Register("t1", cmd, logPath, cancel, "test description", "test command")
	if err != nil {
		t.Fatal(err)
	}

	// Wait for notification
	select {
	case <-tm.NotifyChan():
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for task completion")
	}

	// Check if log file was recreated
	content, ok := fs.files[logPath]
	if !ok {
		t.Fatalf("expected log file %s to be recreated", logPath)
	}
	if string(content) != "secret output" {
		t.Errorf("expected content 'secret output', got %q", string(content))
	}

	notifs := tm.Drain()
	if len(notifs) != 1 {
		t.Fatal("expected 1 notification")
	}
	if !strings.Contains(notifs[0], fmt.Sprintf("<log-file>%s</log-file>", logPath)) {
		t.Errorf("notification missing log path: %s", notifs[0])
	}
}

type mockBashFileSystem struct {
	files map[string][]byte
}

func (m *mockBashFileSystem) Open(path string) (domain.File, error) {
	return nil, nil // Not used in this test
}

func (m *mockBashFileSystem) CreateAtomic(path string) (io.WriteCloser, error) {
	return &mockBashWriteCloser{path: path, fs: m}, nil
}

type mockBashWriteCloser struct {
	path string
	fs   *mockBashFileSystem
	buf  []byte
}

func (m *mockBashWriteCloser) Write(p []byte) (n int, err error) {
	m.buf = append(m.buf, p...)
	return len(p), nil
}

func (m *mockBashWriteCloser) Close() error {
	m.fs.files[m.path] = m.buf
	return nil
}

func TestTaskManager_HandleCompletion_Constants(t *testing.T) {
	tm := NewTaskManager(nil)

	t.Run("zero exit code -> execution completed", func(t *testing.T) {
		tm.handleCompletion("t1", &executor.Result{ExitCode: 0}, nil, "/tmp/t1.log")
		notif := tm.Drain()[0]
		assert.Contains(t, notif, "<status>execution completed</status>")
		assert.Contains(t, notif, "<exit-code>0</exit-code>")
	})

	t.Run("non-zero exit code -> still execution completed", func(t *testing.T) {
		tm.handleCompletion("t2", &executor.Result{ExitCode: 1}, nil, "/tmp/t2.log")
		notif := tm.Drain()[0]
		assert.Contains(t, notif, "<status>execution completed</status>")
		assert.Contains(t, notif, "<exit-code>1</exit-code>")
	})

	t.Run("context cancelled -> ToolErrorCancelled", func(t *testing.T) {
		tm.handleCompletion("t3", &executor.Result{ExitCode: -1}, context.Canceled, "/tmp/t3.log")
		notif := tm.Drain()[0]
		assert.Contains(t, notif, fmt.Sprintf("<status>%s</status>", domain.ToolErrorCancelled))
	})

	t.Run("context deadline exceeded -> ToolErrorTimedOut as status", func(t *testing.T) {
		tm.handleCompletion("t4", &executor.Result{ExitCode: -1}, context.DeadlineExceeded, "/tmp/t4.log")
		notif := tm.Drain()[0]
		assert.Contains(t, notif, fmt.Sprintf("<status>%s</status>", domain.ToolErrorTimedOut))
		assert.NotContains(t, notif, "<timedout>")
	})

	t.Run("system error -> ToolErrorFailed with <error> tag", func(t *testing.T) {
		tm.handleCompletion("t5", &executor.Result{ExitCode: -1}, fmt.Errorf("system crash"), "/tmp/t5.log")
		notif := tm.Drain()[0]
		// Red Phase: This currently has <status>failed</status>
		assert.Contains(t, notif, fmt.Sprintf("<status>%s</status>", domain.ToolErrorFailed))
		assert.Contains(t, notif, "<error>system crash</error>")
	})
}
