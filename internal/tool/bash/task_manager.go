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
	mu         sync.Mutex
	fs         fileSystem
	tasks      map[string]*bashTask
	doneQueue  []string
	notifyChan chan struct{}
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
		if errors.Is(err, context.DeadlineExceeded) {
			status = domain.ToolErrorTimedOut
		} else if errors.Is(err, context.Canceled) {
			status = domain.ToolErrorCancelled
		} else {
			status = domain.ToolErrorFailed
			errDetails = err
		}
	}

	errorXML := ""
	if errDetails != nil {
		errorXML = fmt.Sprintf("\n  <error>%v</error>", errDetails)
	}

	exitCode := 0
	if res != nil {
		exitCode = res.ExitCode
	}

	effectiveLogPath := logPath
	if res != nil {
		if res.LogPath != "" {
			effectiveLogPath = res.LogPath
		} else if res.Stdout != "" && m.fs != nil {
			// Reconstruct log file if it was deleted by executor (<16kb)
			if fl, err := m.fs.CreateAtomic(logPath); err == nil {
				_, _ = fl.Write([]byte(res.Stdout))
				_ = fl.Close()
			}
		}
	}
	logFileXML := fmt.Sprintf("\n  <log-file>%s</log-file>", effectiveLogPath)

	// Format XML notification
	xml := fmt.Sprintf(`<task-notification>
  <task-id>%s</task-id>
  <status>%s</status>
  <description>%s</description>
  <command>%s</command>
  <exit-code>%d</exit-code>%s%s
</task-notification>`, id, status, desc, command, exitCode, errorXML, logFileXML)

	m.doneQueue = append(m.doneQueue, xml)
	delete(m.tasks, id)

	// Signal listeners (Sleep tool)
	close(m.notifyChan)
	m.notifyChan = make(chan struct{})
}

// Drain returns and clears the pending notification queue.
func (m *TaskManager) Drain() []string {
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

// List returns a summary of all active tasks.
func (m *TaskManager) List() []TaskInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	var tasks []TaskInfo
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
