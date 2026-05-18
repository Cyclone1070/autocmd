package bash

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
)

// TaskInfo contains public information about a background task.
type TaskInfo struct {
	ID                   string `json:"id"`
	Description          string `json:"description"`
	Command              string `json:"command"`
	SecondsSinceActivity int    `json:"seconds_since_activity"`
}

// TaskManager manages background processes promoted from BashTool.
type TaskManager struct {
	fs         fileSystem
	tasks      map[string]*bashTask
	notifyChan chan struct{}
	doneQueue  []domain.TaskResult
	mu         sync.Mutex
}

// bashTask tracks an individual background process.
type bashTask struct {
	id          string
	description string
	command     string
	cmd         *executor.StreamingCmd
	cancel      context.CancelFunc
	logPath     string
}

// NewTaskManager creates a new task manager.
func NewTaskManager(fs fileSystem) *TaskManager {
	return &TaskManager{
		fs:         fs,
		tasks:      make(map[string]*bashTask),
		notifyChan: make(chan struct{}),
	}
}

// Register takes control of an already running process and tracks it in the background.
func (m *TaskManager) Register(id string, proc *executor.StreamingCmd, logPath string, cancel context.CancelFunc, description, command string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	task := &bashTask{
		id:          id,
		description: description,
		command:     command,
		cmd:         proc,
		cancel:      cancel,
		logPath:     logPath,
	}
	m.tasks[id] = task

	// Start a goroutine to wait for completion
	go func() {
		res, err := proc.Wait()
		m.handleCompletion(id, res, err, logPath)
	}()

	return nil
}

// handleCompletion processes the exit of a background task.
func (m *TaskManager) handleCompletion(id string, res *executor.Result, err error, logPath string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get task info before deleting it
	task, ok := m.tasks[id]
	desc := ""
	command := ""
	if ok {
		desc = task.description
		command = task.command
	}

	status := "execution completed"
	var errDetails error
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			status = domain.ToolErrorTimedOut
		case errors.Is(err, context.Canceled):
			status = domain.ToolErrorCancelled
		default:
			status = domain.ToolErrorFailed
			errDetails = err
		}
	}

	exitCode := 0
	if res != nil {
		exitCode = res.ExitCode
	}

	effectiveLogPath := logPath
	if res != nil && res.LogPath != "" {
		effectiveLogPath = res.LogPath
	}
	result := domain.TaskResult{
		ID:          id,
		Status:      status,
		Description: desc,
		Command:     command,
		ExitCode:    exitCode,
		LogPath:     effectiveLogPath,
	}
	if errDetails != nil {
		result.Error = errDetails.Error()
	}

	m.doneQueue = append(m.doneQueue, result)
	delete(m.tasks, id)

	// Signal listeners (Sleep tool)
	close(m.notifyChan)
	m.notifyChan = make(chan struct{})
}

// Drain returns and clears the pending notification queue.
func (m *TaskManager) Drain() []domain.TaskResult {
	m.mu.Lock()
	defer m.mu.Unlock()

	q := m.doneQueue
	m.doneQueue = nil
	return q
}

// NotifyChan returns a signal channel that closes when any background task completes.
func (m *TaskManager) NotifyChan() <-chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.notifyChan
}

// HasPending returns true if there are pending notifications in the queue.
func (m *TaskManager) HasPending() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.doneQueue) > 0
}

// HasRunning returns true if there are still active background tasks.
func (m *TaskManager) HasRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.tasks) > 0
}

// Stop terminates a background task by ID.
func (m *TaskManager) Stop(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}

	task.cancel()
	return nil
}

// StopAll terminates all active background tasks.
func (m *TaskManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, task := range m.tasks {
		task.cancel()
	}
}

// List returns a summary of all active tasks.
func (m *TaskManager) List() []TaskInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	tasks := make([]TaskInfo, 0, len(m.tasks))
	for id, t := range m.tasks {
		lastActivity := t.cmd.LastActivityAt()
		secondsSince := -1
		if !lastActivity.IsZero() {
			secondsSince = int(time.Since(lastActivity).Seconds())
		}

		tasks = append(tasks, TaskInfo{
			ID:                   id,
			Description:          t.description,
			Command:              t.command,
			SecondsSinceActivity: secondsSince,
		})
	}
	return tasks
}
