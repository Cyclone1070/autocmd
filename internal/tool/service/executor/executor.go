package executor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	"github.com/Cyclone1070/iav/internal/tool/helper/follow"
)

const (
	defaultMaxOutputSize       = 500 * 1024 * 1024 // 500MB default limit
	DefaultSmartDrainThreshold = 16 * 1024        // 16KB
	DefaultBinarySampleSize    = 8000             // 8KB sample for binary detection
	DefaultBufferSize          = 4096             // 4KB standard buffer
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
	return exec.CommandContext(ctx, name, args...)
}

// Result represents the outcome of a command execution.
type Result struct {
	Stdout    string
	ExitCode  int
	LogPath   string
	Truncated bool
}

// StreamingCmd represents a running command with real-time output streaming.
type StreamingCmd struct {
	output io.Reader

	once    sync.Once
	result  *Result
	err     error
	wait    func() (*Result, error)
	logPath string
	id      string

	mu             sync.Mutex
	lastActivityAt time.Time
}

func NewStreamingCmd(id string, output io.Reader, wait func() (*Result, error), logPath string) *StreamingCmd {
	return &StreamingCmd{
		id:      id,
		output:  output,
		wait:    wait,
		logPath: logPath,
	}
}

func (s *StreamingCmd) ID() string {
	return s.id
}

func (s *StreamingCmd) Output() io.Reader {
	return s.output
}

func (s *StreamingCmd) LogPath() string {
	return s.logPath
}

func (s *StreamingCmd) Wait() (*Result, error) {
	s.once.Do(func() {
		s.result, s.err = s.wait()
	})
	return s.result, s.err
}

func (s *StreamingCmd) LastActivityAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastActivityAt
}

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

type OSCommandExecutor struct {
	fs            fileSystem
	maxOutputSize int64

	SmartDrainThreshold int64
	DefaultTimeout      time.Duration
	killer              signalKiller
	commander           commandFactory
}

func NewOSCommandExecutor(fs fileSystem) *OSCommandExecutor {
	return &OSCommandExecutor{
		fs:                  fs,
		maxOutputSize:       defaultMaxOutputSize,
		SmartDrainThreshold: DefaultSmartDrainThreshold,
		killer:              &osSignalKiller{},
		commander:           &osCommandFactory{},
	}
}

func (f *OSCommandExecutor) Run(ctx context.Context, command string, dir string, enableLogging bool) (*Result, error) {
	s, err := f.RunStreaming(ctx, command, dir, enableLogging)
	if err != nil {
		return nil, err
	}
	return s.Wait()
}

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

	_ = f.fs.MkdirAll(logDir, 0o755)
	const idLen = 8
	for range 5 {
		tmpID := randomShortID(idLen)
		tmpPath := filepath.Join(logDir, tmpID+".output.log")
		fl, err := f.fs.CreateAtomic(tmpPath)
		if err == nil {
			logFile = fl
			logPath = tmpPath
			finalID = tmpID
			break
		}
	}

	if logFile == nil {
		return nil, fmt.Errorf("failed to create log file")
	}

	follower := follow.NewFollower(f.fs, logPath)
	
	var wg sync.WaitGroup
	wg.Add(2)

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
			shouldReturnStdout := !enableLogging || (err == nil && info.Size() < f.SmartDrainThreshold)

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
		return res, execErr
	}

	sc = NewStreamingCmd(finalID, follower, waitFn, logPath)

	var bytesWritten int64
	var bytesMu sync.Mutex
	copyStream := func(src io.Reader) {
		defer wg.Done()
		buf := make([]byte, 4096)
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

func randomShortID(length int) string {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		panic(fmt.Sprintf("failed to generate random ID: %v", err))
	}
	return hex.EncodeToString(bytes)
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
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
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
	var env []string
	for k, v := range envMap {
		env = append(env, k+"="+v)
	}
	return env
}
