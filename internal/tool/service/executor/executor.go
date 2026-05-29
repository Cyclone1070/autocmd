// Package executor provides functionality for running OS commands with streaming output.
package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/randutil"
	"github.com/Cyclone1070/iav/internal/tool/helper/follow"
)

const (
	defaultMaxOutputSize       = 500 * 1024 * 1024 // 500MB default limit
	defaultSmartDrainThreshold = 16 * 1024         // 16KB
	defaultBufferSize          = 4096              // 4KB standard buffer
	numPipes                   = 2                 // stdout, stderr

	kvParts = 2
)

type signalKiller interface {
	Kill(pid int, sig syscall.Signal) error
}

type osSignalKiller struct{}

func (s *osSignalKiller) Kill(pid int, sig syscall.Signal) error {
	return syscall.Kill(pid, sig)
}

type commandFactory interface {
	Command(ctx context.Context, name string, args ...string) *exec.Cmd
}

type osCommandFactory struct{}

func (f *osCommandFactory) Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	// #nosec G204, G702 - Intentional subprocess execution for tool service
	return exec.CommandContext(ctx, name, args...)
}

// Result represents the outcome of a command execution.
type Result struct {
	Stdout    string
	LogPath   string
	ExitCode  int
	Truncated bool
}

// StreamingCmd represents a running command with real-time output streaming.
type StreamingCmd struct {
	lastActivityAt      time.Time
	output              io.Reader
	err                 error
	result              *Result
	wait                func() (*Result, error)
	logPath             string
	id                  string
	autoCleanupDisabled bool
	once                sync.Once
	mu                  sync.Mutex
}

// NewStreamingCmd creates a new StreamingCmd instance.
func NewStreamingCmd(id string, output io.Reader, wait func() (*Result, error), logPath string) *StreamingCmd {
	return &StreamingCmd{
		id:      id,
		output:  output,
		wait:    wait,
		logPath: logPath,
	}
}

// ID returns the unique identifier for the command.
func (s *StreamingCmd) ID() string {
	return s.id
}

// DisableAutoCleanup prevents the command from deleting the log file if it is small.
func (s *StreamingCmd) DisableAutoCleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.autoCleanupDisabled = true
}

// Output returns a reader for the command's streaming output.
func (s *StreamingCmd) Output() io.Reader {
	return s.output
}

// LogPath returns the path to the file where output is being logged.
func (s *StreamingCmd) LogPath() string {
	return s.logPath
}

// Wait blocks until the command completes and returns the result.
func (s *StreamingCmd) Wait() (*Result, error) {
	s.once.Do(func() {
		s.result, s.err = s.wait()
	})
	return s.result, s.err
}

// LastActivityAt returns the time of the most recent output from the command.
func (s *StreamingCmd) LastActivityAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastActivityAt
}

// UpdateActivity updates the last activity timestamp to the current time.
func (s *StreamingCmd) UpdateActivity() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastActivityAt = time.Now()
}

type fileSystem interface {
	MkdirAll(path string, perm os.FileMode) error
	CreateAtomic(path string) (io.WriteCloser, error)
	Remove(path string) error
	Stat(path string) (os.FileInfo, error)
	Open(path string) (domain.File, error)
}

// OSCommandExecutor implements the command execution service using the OS shell.
type OSCommandExecutor struct {
	fs                  fileSystem
	killer              signalKiller
	commander           commandFactory
	maxOutputSize       int64
	SmartDrainThreshold int64
	DefaultTimeout      time.Duration
}

// NewOSCommandExecutor creates a new OSCommandExecutor with the provided filesystem.
func NewOSCommandExecutor(fs fileSystem) *OSCommandExecutor {
	return &OSCommandExecutor{
		fs:                  fs,
		maxOutputSize:       defaultMaxOutputSize,
		SmartDrainThreshold: defaultSmartDrainThreshold,
		killer:              &osSignalKiller{},
		commander:           &osCommandFactory{},
	}
}

// Run executes a command and waits for its completion.
func (f *OSCommandExecutor) Run(ctx context.Context, command string, dir string, enableLogging bool) (*Result, error) {
	s, err := f.RunStreaming(ctx, command, dir, enableLogging)
	if err != nil {
		return nil, err
	}
	return s.Wait()
}

// RunStreaming starts a command and returns a StreamingCmd for real-time output.
func (f *OSCommandExecutor) RunStreaming(ctx context.Context, command string, dir string, enableLogging bool) (sc *StreamingCmd, err error) {
	// Fallback timeout if no deadline is set in context (None by default now)
	var cancel context.CancelFunc
	// We no longer enforce a default 30m timeout here to allow indefinite background tasks

	// Always sanitize environment for security using the secure base
	envMap := sanitizeEnv()
	sanitizedEnv := envMapToSlice(envMap)

	// On Unix-like systems, we execute via the detected shell with -l -c
	// to ensure environment parity (profiles, aliases) and POSIX features.
	shell := envMap["SHELL"]
	if shell == "" {
		shell = os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/bash"
		}
	}

	// Build a sanitization prefix that runs INSIDE the shell after profiles are sourced.
	// This ensures our policies (like TERM) survive the login shell's initialization.
	sanitizationPrefix := "export TERM=dumb; "

	cmd := f.commander.Command(ctx, shell, "-l", "-c", sanitizationPrefix+command)
	cmd.Dir = dir
	cmd.Env = sanitizedEnv
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	cmd.Cancel = func() error {
		// Negative PID kills the process group in Unix
		return f.killer.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("command failed to start: %w", err)
	}

	var logFile io.WriteCloser
	var logPath string
	var finalID string

	var logDir string
	if enableLogging {
		if sessionID, ok := domain.GetSessionID(ctx); ok && sessionID != "" {
			home, _ := os.UserHomeDir()
			logDir = filepath.Join(home, domain.ConfigBaseDir, domain.AppName, "sessions", sessionID)
		}
	}

	if logDir == "" {
		// If logging is disabled or sessionID is missing, use a temporary directory
		logDir = os.TempDir()
	}

	_ = f.fs.MkdirAll(logDir, domain.DefaultDirPerm)
	var tmpID string
	for i := range domain.MaxCollisionRetries {
		tmpID = randutil.ShortID(domain.ShortIDLength)
		tmpPath := filepath.Join(logDir, tmpID+".output.log")
		fl, err := f.fs.CreateAtomic(tmpPath)
		if err == nil {
			logFile = fl
			logPath = tmpPath
			break
		}
		if i == domain.MaxCollisionRetries-1 {
			return nil, fmt.Errorf("failed to generate unique log ID: failed to generate unique ID after 100 attempts")
		}
	}
	finalID = tmpID

	if logFile == nil {
		return nil, fmt.Errorf("failed to create log file")
	}

	follower := follow.NewFollower(f.fs, logPath)

	var wg sync.WaitGroup
	wg.Add(numPipes)

	waitFn := func() (*Result, error) {
		if cancel != nil {
			defer cancel()
		}
		execErr := cmd.Wait()
		wg.Wait()
		if logFile != nil {
			_ = logFile.Close()
		}
		follower.Stop()

		res := &Result{
			ExitCode: f.getExitCode(execErr),
		}

		if logPath != "" {
			info, err := f.fs.Stat(logPath)
			shouldReturnStdout := !enableLogging || (!sc.autoCleanupDisabled && err == nil && info.Size() < f.SmartDrainThreshold)

			if shouldReturnStdout {
				if fl, err := f.fs.Open(logPath); err == nil {
					content, _ := io.ReadAll(fl)
					_ = fl.Close()
					res.Stdout = string(content)
					_ = f.fs.Remove(logPath)
				}
			} else {
				res.LogPath = logPath
				res.Stdout = ""
			}
		}

		if ctx.Err() != nil {
			return res, ctx.Err()
		}

		var ee *exec.ExitError
		if errors.As(execErr, &ee) && ee.ExitCode() >= 0 {
			execErr = nil
		}
		return res, execErr
	}

	sc = NewStreamingCmd(finalID, follower, waitFn, logPath)

	var bytesWritten int64
	var bytesMu sync.Mutex
	copyStream := func(src io.Reader) {
		defer wg.Done()
		buf := make([]byte, defaultBufferSize)
		for {
			n, err := src.Read(buf)
			if n > 0 {
				chunk := buf[:n]

				// Watchdog check
				bytesMu.Lock()
				bytesWritten += int64(len(chunk))
				if bytesWritten > f.maxOutputSize {
					bytesMu.Unlock()
					_ = f.killer.Kill(-cmd.Process.Pid, syscall.SIGKILL)
					return
				}
				bytesMu.Unlock()

				if logFile != nil {
					_, _ = logFile.Write(chunk)
					follower.Poke()
				}
				sc.UpdateActivity()
			}
			if err != nil {
				break
			}
		}
	}

	go copyStream(stdoutPipe)
	go copyStream(stderrPipe)

	return sc, nil
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

var envWhitelist = map[string]bool{
	"PATH":  true,
	"HOME":  true,
	"USER":  true,
	"LANG":  true,
	"SHELL": true,
	"TERM":  true,
}

func sanitizeEnv() map[string]string {
	env := os.Environ()

	// Use a map for deduplication and easy override
	envMap := make(map[string]string)
	for _, e := range env {
		parts := strings.SplitN(e, "=", kvParts)
		if len(parts) == kvParts {
			if envWhitelist[parts[0]] {
				envMap[parts[0]] = parts[1]
			}
		}
	}

	// Always force TERM=dumb for clean AI-parseable output
	envMap["TERM"] = "dumb"

	return envMap
}

func envMapToSlice(envMap map[string]string) []string {
	env := make([]string, 0, len(envMap))
	for k, v := range envMap {
		env = append(env, k+"="+v)
	}
	return env
}
