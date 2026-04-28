package executor

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/stretchr/testify/assert"
)

type mockFileSystem struct {
	files        map[string]*bytes.Buffer
	removedFiles []string
	createdPaths []string
}

func (m *mockFileSystem) MkdirAll(_ string, _ os.FileMode) error {
	return nil
}

func (m *mockFileSystem) CreateAtomic(path string) (io.WriteCloser, error) {
	m.createdPaths = append(m.createdPaths, path)
	if m.files == nil {
		m.files = make(map[string]*bytes.Buffer)
	}
	buf := &bytes.Buffer{}
	m.files[path] = buf
	return &mockWriteCloser{Buffer: buf}, nil
}

func (m *mockFileSystem) Remove(path string) error {
	m.removedFiles = append(m.removedFiles, path)
	delete(m.files, path)
	return nil
}

func (m *mockFileSystem) Stat(path string) (os.FileInfo, error) {
	if _, ok := m.files[path]; ok {
		return &mockFileInfo{size: int64(m.files[path].Len())}, nil
	}
	return nil, os.ErrNotExist
}

func (m *mockFileSystem) Open(path string) (domain.File, error) {
	if buf, ok := m.files[path]; ok {
		return &mockFile{Reader: bytes.NewReader(buf.Bytes()), size: int64(buf.Len())}, nil
	}
	return nil, os.ErrNotExist
}

type mockSignalKiller struct {
	killedPid int
	killedSig syscall.Signal
}

func (m *mockSignalKiller) Kill(pid int, sig syscall.Signal) error {
	m.killedPid = pid
	m.killedSig = sig
	// Actually kill the process (negating back to positive if needed)
	// so the test command terminates and Wait() returns.
	target := pid
	if target < 0 {
		target = -target
	}
	return syscall.Kill(target, sig)
}

type mockCommandFactory struct {
	gotName string
	gotArgs []string
}

func (m *mockCommandFactory) Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	m.gotName = name
	m.gotArgs = args
	// We still return a real Cmd so Start/Wait don't panic, but we don't care about its execution
	return exec.CommandContext(ctx, "true")
}

type mockFile struct {
	*bytes.Reader
	size int64
}

func (m *mockFile) Close() error { return nil }
func (m *mockFile) Stat() (os.FileInfo, error) {
	return &mockFileInfo{size: m.size}, nil
}

type mockFileInfo struct {
	os.FileInfo
	size int64
}

func (m *mockFileInfo) Size() int64        { return m.size }
func (m *mockFileInfo) IsDir() bool        { return false }
func (m *mockFileInfo) Name() string       { return "mock" }
func (m *mockFileInfo) Mode() os.FileMode  { return 0 }
func (m *mockFileInfo) ModTime() time.Time { return time.Now() }
func (m *mockFileInfo) Sys() any           { return nil }

type mockWriteCloser struct {
	*bytes.Buffer
}

func (m *mockWriteCloser) Close() error {
	return nil
}

func TestOSCommandExecutor_StringCommand(t *testing.T) {
	exec := NewOSCommandExecutor(&mockFileSystem{})

	// Test that it can parse and run a simple string command
	res, err := exec.Run(context.Background(), "echo 'hello world'", "", false)
	if err != nil {
		t.Fatalf("Failed to run string command: %v", err)
	}

	if !strings.Contains(res.Stdout, "hello world") {
		t.Errorf("Expected 'hello world', got %q", res.Stdout)
	}
}

func TestOSCommandExecutor_QuotedArgs(t *testing.T) {
	exec := NewOSCommandExecutor(&mockFileSystem{})

	// Test complex quoting
	res, err := exec.Run(context.Background(), "printf '%s %s' 'arg one' \"arg two\"", "", false)
	if err != nil {
		t.Fatalf("Failed to run quoted command: %v", err)
	}

	if res.Stdout != "arg one arg two" {
		t.Errorf("Expected 'arg one arg two', got %q", res.Stdout)
	}
}

func TestStreamingCmd_ActivityTracking(t *testing.T) {
	reader := bytes.NewReader([]byte("some data"))
	waitFn := func() (*Result, error) {
		return &Result{ExitCode: 0}, nil
	}

	cmd := NewStreamingCmd("test-id", reader, waitFn, "test.log")

	// Initially activity should be zero
	if !cmd.LastActivityAt().IsZero() {
		t.Errorf("Expected initial LastActivityAt to be zero, got %v", cmd.LastActivityAt())
	}

	// Manual update
	now := time.Now()
	cmd.UpdateActivity()

	activityAt := cmd.LastActivityAt()
	if activityAt.Before(now) {
		t.Errorf("Expected LastActivityAt to be at or after %v, got %v", now, activityAt)
	}
}

func TestOSCommandExecutor_InternalLogPath(t *testing.T) {
	fs := &mockFileSystem{}
	exec := NewOSCommandExecutor(fs)
	sessionID := "test-session-123"
	ctx := domain.WithSessionID(context.Background(), sessionID)

	t.Run("Logging enabled builds path internally", func(t *testing.T) {
		fs.createdPaths = nil
		_, err := exec.Run(ctx, "echo 'hello'", "", true)
		if err != nil {
			t.Fatal(err)
		}

		// Verify that a path was created containing the session ID and following the expected pattern
		found := false
		for _, p := range fs.createdPaths {
			if strings.Contains(p, "sessions") && strings.Contains(p, sessionID) && strings.HasSuffix(p, ".output.log") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected a log path containing 'sessions' and %q, but got none of: %v", sessionID, fs.createdPaths)
		}
	})
}

func TestStreamingCmd_ID(t *testing.T) {
	fs := &mockFileSystem{}
	exec := NewOSCommandExecutor(fs)

	sc, err := exec.RunStreaming(context.Background(), "echo test", "", true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = sc.Wait() }()

	id := sc.ID()
	if id == "" {
		t.Error("Expected non-empty ID from StreamingCmd")
	}

	if !strings.Contains(sc.LogPath(), id) {
		t.Errorf("Expected LogPath %q to contain ID %q", sc.LogPath(), id)
	}
}

func TestOSCommandExecutor_ProcessGroupKill(t *testing.T) {
	fs := &mockFileSystem{}
	killer := &mockSignalKiller{}
	exec := NewOSCommandExecutor(fs)
	exec.killer = killer

	ctx, cancel := context.WithCancel(context.Background())

	// Start a long running command
	sc, err := exec.RunStreaming(ctx, "sleep 100", "", false)
	if err != nil {
		t.Fatal(err)
	}

	// Trigger cancellation
	cancel()
	_, _ = sc.Wait()

	// Verify that the killer was called with the NEGATIVE PID (group kill)
	if killer.killedPid >= 0 {
		t.Errorf("Expected to kill process group (negative PID), but got %d", killer.killedPid)
	}
}

func TestOSCommandExecutor_ShellResolution(t *testing.T) {
	_ = os.Setenv("SHELL", "/bin/custom_shell")
	defer func() { _ = os.Unsetenv("SHELL") }()

	fs := &mockFileSystem{}
	commander := &mockCommandFactory{}
	exec := NewOSCommandExecutor(fs)
	exec.commander = commander

	ctx := context.Background()
	_, _ = exec.RunStreaming(ctx, "my_command", "", false)

	if commander.gotName != "/bin/custom_shell" {
		t.Errorf("Expected shell /bin/custom_shell, got %q", commander.gotName)
	}

	prefix := "export TERM=dumb; "
	expectedArgs := []string{"-l", "-c", prefix + "my_command"}
	if !reflect.DeepEqual(commander.gotArgs, expectedArgs) {
		t.Errorf("Expected args %v, got %v", expectedArgs, commander.gotArgs)
	}
}

func TestOSCommandExecutor_EnvironmentSanitization(t *testing.T) {
	// Setup a dirty environment
	_ = os.Setenv("SECRET_KEY", "highly_sensitive_data")
	_ = os.Setenv("PATH", "/usr/bin:/bin")
	defer func() { _ = os.Unsetenv("SECRET_KEY") }()
	defer func() { _ = os.Unsetenv("PATH") }()

	fs := &mockFileSystem{}
	exec := NewOSCommandExecutor(fs)

	// Run a command that prints ENV
	// We use strings.Contains because POSIX env output format is VAR=VAL
	res, err := exec.Run(context.Background(), "env", "", false)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Verify Whitelist: SECRET_KEY should NOT be present
	if strings.Contains(res.Stdout, "SECRET_KEY") {
		t.Errorf("Security Leak: SECRET_KEY was found in process output!")
	}

	// 2. Verify TERM: Should be dumb
	if !strings.Contains(res.Stdout, "TERM=dumb") {
		t.Errorf("TERM was not set to dumb, got output: %q", res.Stdout)
	}
}

func TestOSCommandExecutor_OutputSizeWatchdog(t *testing.T) {
	// Use real OS file system
	osFS := fs.NewOSFileSystem(-1)
	exec := NewOSCommandExecutor(osFS)
	exec.maxOutputSize = 100 // 100 bytes

	ctx := context.Background()
	// Produces infinite output
	_, err := exec.Run(ctx, "yes", ".", true)

	assert.Error(t, err)
	// On most systems, SIGKILL results in "signal: killed"
	assert.Contains(t, err.Error(), "killed")
}

func TestOSCommandExecutor_NoFallbackTimeout(t *testing.T) {
	// Verify that we can run for longer than 30m if no deadline is set
	// We'll just test for 1s to prove there is no short-circuit.
	osFS := fs.NewOSFileSystem(-1)
	exec := NewOSCommandExecutor(osFS)

	ctx := context.Background()
	start := time.Now()
	// Command that sleeps for 1.5s
	_, err := exec.Run(ctx, "sleep 1.5", ".", true)
	duration := time.Since(start)

	assert.NoError(t, err)
	assert.GreaterOrEqual(t, duration, 1*time.Second)
}
