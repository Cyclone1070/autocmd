package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Clock defines a way to handle time-based operations in a testable way.
type Clock interface {
	After(d time.Duration) <-chan time.Time
}

// RealClock implements Clock using the standard time package.
type RealClock struct{}

func (RealClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

const (
	defaultMaxOutputSize            = 10 * 1024 * 1024 // 10MB
	defaultDockerGracefulShutdownMs = 2000
)

// Result represents the outcome of a command execution.
// For RunStreaming, Stdout contains combined stdout/stderr output and Stderr is empty.
type Result struct {
	Stdout    string
	Stderr    string
	ExitCode  int
	Truncated bool
}

// StreamingCmd represents a running command with real-time output streaming.
// Use Output() to get the combined stdout/stderr stream for UI consumption.
// Use Wait() to block until the command completes and get the final result.
// Wait is safe to call multiple times; subsequent calls return the cached result.
type StreamingCmd struct {
	output io.Reader

	once   sync.Once
	result *Result
	err    error
	wait   func() (*Result, error)
}

// Output returns the combined stdout/stderr stream for real-time consumption.
func (s *StreamingCmd) Output() io.Reader {
	return s.output
}

// Wait blocks until the command completes and returns the result.
// Safe to call multiple times; subsequent calls return the cached result.
func (s *StreamingCmd) Wait() (*Result, error) {
	s.once.Do(func() {
		s.result, s.err = s.wait()
	})
	return s.result, s.err
}

// NewStreamingCmd creates a new StreamingCmd with the given output reader and wait function.
// This is primarily used for testing; production code uses RunStreaming.
func NewStreamingCmd(output io.Reader, wait func() (*Result, error)) *StreamingCmd {
	return &StreamingCmd{
		output: output,
		wait:   wait,
	}
}

// OSCommandExecutor implements command execution using os/exec for real system commands.
type OSCommandExecutor struct {
	maxOutputSize            int64
	dockerGracefulShutdownMs int
	clock                    Clock
}

// NewOSCommandExecutor creates a new OSCommandExecutor.
func NewOSCommandExecutor() *OSCommandExecutor {
	return &OSCommandExecutor{
		maxOutputSize:            defaultMaxOutputSize,
		dockerGracefulShutdownMs: defaultDockerGracefulShutdownMs,
		clock:                    RealClock{},
	}
}

// Run executes a command and returns the result. It buffers output internally.
func (f *OSCommandExecutor) Run(ctx context.Context, command []string, dir string, env []string) (*Result, error) {
	if len(command) == 0 {
		return nil, os.ErrInvalid
	}

	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = nil

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("command %s failed to start: %w", command[0], err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("command %s failed to start: %w", command[0], err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("command %s failed to start: %w", command[0], err)
	}

	stdoutStr, stderrStr, truncated := f.collectOutput(stdoutPipe, stderrPipe)

	err = cmd.Wait()
	exitCode := 0
	if err != nil {
		exitCode = f.getExitCode(err)
	}

	return &Result{
		Stdout:    stdoutStr,
		Stderr:    stderrStr,
		ExitCode:  exitCode,
		Truncated: truncated,
	}, nil
}

// RunWithTimeout executes a command with a timeout and graceful shutdown.
func (f *OSCommandExecutor) RunWithTimeout(ctx context.Context, command []string, dir string, env []string, timeout time.Duration) (*Result, error) {
	if len(command) == 0 {
		return nil, os.ErrInvalid
	}

	// We don't use CommandContext's timeout here because we want to handle graceful shutdown
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = nil

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("command %s failed to start: %w", command[0], err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("command %s failed to start: %w", command[0], err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("command %s failed to start: %w", command[0], err)
	}

	// Start output collection concurrently so it doesn't block the timeout select
	var stdoutStr, stderrStr string
	var truncated bool
	collectDone := make(chan struct{})
	go func() {
		stdoutStr, stderrStr, truncated = f.collectOutput(stdoutPipe, stderrPipe)
		close(collectDone)
	}()

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var execErr error
	select {
	case err := <-done:
		execErr = err
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		execErr = ctx.Err()
	case <-f.clock.After(timeout):
		if cmd.Process != nil {
			// Try graceful shutdown
			_ = cmd.Process.Signal(os.Interrupt)
			select {
			case <-done:
				execErr = ErrTimeout
			case <-f.clock.After(time.Duration(f.dockerGracefulShutdownMs) * time.Millisecond):
				_ = cmd.Process.Kill()
				execErr = ErrTimeout
			}
		}
	}

	// Wait for output collection to finish (it should when cmd.Wait/Kill happens)
	<-collectDone

	exitCode := 0
	if execErr != nil {
		exitCode = f.getExitCode(execErr)
		if errors.Is(execErr, ErrTimeout) {
			exitCode = -1
		}
	}

	res := &Result{
		Stdout:    stdoutStr,
		Stderr:    stderrStr,
		ExitCode:  exitCode,
		Truncated: truncated,
	}

	// Only return error for infrastructure failures (timeout or context cancelled)
	// Exit code failures are handled within the Result.
	if errors.Is(execErr, ErrTimeout) ||
		errors.Is(execErr, context.Canceled) ||
		errors.Is(execErr, context.DeadlineExceeded) {
		return res, execErr
	}

	return res, nil
}

// RunStreaming executes a command with streaming output for real-time UI display.
// It returns immediately after starting the command. The caller reads from Output
// and calls Wait() to get the final result when done.
// The timeout is measured from when the command starts, not when Wait() is called.
func (f *OSCommandExecutor) RunStreaming(ctx context.Context, command []string, dir string, env []string, timeout time.Duration) (*StreamingCmd, error) {
	if len(command) == 0 {
		return nil, os.ErrInvalid
	}

	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = nil

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("command %s failed to start: %w", command[0], err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("command %s failed to start: %w", command[0], err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("command %s failed to start: %w", command[0], err)
	}

	// Start timeout timer immediately after command starts
	timeoutCh := f.clock.After(timeout)

	// Combined output pipe for streaming to UI
	pr, pw := io.Pipe()

	// Buffer to capture output for final Result
	maxBytes := int(f.maxOutputSize)
	collector := newCollector(maxBytes, 8000)
	var collectorMu sync.Mutex

	// Copy stdout and stderr to both the pipe (UI) and collector (Result)
	var wg sync.WaitGroup
	wg.Add(2)

	copyStream := func(src io.Reader) {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, readErr := src.Read(buf)
			if n > 0 {
				chunk := buf[:n]
				_, _ = pw.Write(chunk)
				collectorMu.Lock()
				_, _ = collector.Write(chunk)
				collectorMu.Unlock()
			}
			if readErr != nil {
				break
			}
		}
	}

	go copyStream(stdoutPipe)
	go copyStream(stderrPipe)

	// Goroutine to close pipe when streams finish
	go func() {
		wg.Wait()
		_ = pw.Close()
	}()

	// Wait function captures the result (uses timeoutCh started at command start)
	waitFn := func() (*Result, error) {
		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()

		var execErr error
		select {
		case err := <-done:
			execErr = err
		case <-ctx.Done():
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			<-done // Wait for process to actually exit
			execErr = ctx.Err()
		case <-timeoutCh:
			if cmd.Process != nil {
				_ = cmd.Process.Signal(os.Interrupt)
				select {
				case <-done:
				case <-f.clock.After(time.Duration(f.dockerGracefulShutdownMs) * time.Millisecond):
					_ = cmd.Process.Kill()
					<-done
				}
			}
			execErr = ErrTimeout
		}

		// Wait for output collection to finish
		wg.Wait()

		collectorMu.Lock()
		output := collector.String()
		truncated := collector.Truncated()
		collectorMu.Unlock()

		exitCode := f.getExitCode(execErr)
		if errors.Is(execErr, ErrTimeout) {
			exitCode = -1
		}

		res := &Result{
			Stdout:    output,
			Stderr:    "",
			ExitCode:  exitCode,
			Truncated: truncated,
		}

		if errors.Is(execErr, ErrTimeout) ||
			errors.Is(execErr, context.Canceled) ||
			errors.Is(execErr, context.DeadlineExceeded) {
			return res, execErr
		}

		return res, nil
	}

	return &StreamingCmd{
		output: pr,
		wait:   waitFn,
	}, nil
}

func (f *OSCommandExecutor) collectOutput(stdout, stderr io.Reader) (string, string, bool) {
	maxBytes := int(f.maxOutputSize)

	stdoutCollector := newCollector(maxBytes, 8000)
	stderrCollector := newCollector(maxBytes, 8000)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(stdoutCollector, stdout)
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(stderrCollector, stderr)
	}()

	wg.Wait()

	truncated := stdoutCollector.Truncated() || stderrCollector.Truncated()
	return stdoutCollector.String(), stderrCollector.String(), truncated
}

func (f *OSCommandExecutor) getExitCode(err error) int {
	if err == nil {
		return 0
	}
	type exitCoder interface {
		ExitCode() int
	}
	if ec, ok := err.(exitCoder); ok {
		return ec.ExitCode()
	}
	return -1
}
