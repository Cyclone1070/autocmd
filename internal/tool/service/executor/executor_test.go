package executor

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"
	"errors"
	"github.com/Cyclone1070/iav/internal/domain"
)

type mockFileSystem struct {
	files        map[string]*bytes.Buffer
	removedFiles []string
	createdPaths []string
}

func (m *mockFileSystem) MkdirAll(path string, perm os.FileMode) error {
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

func (m *mockFileInfo) Size() int64 { return m.size }
func (m *mockFileInfo) IsDir() bool { return false }
func (m *mockFileInfo) Name() string { return "mock" }
func (m *mockFileInfo) Mode() os.FileMode { return 0 }
func (m *mockFileInfo) ModTime() time.Time { return time.Now() }
func (m *mockFileInfo) Sys() interface{} { return nil }

type mockWriteCloser struct {
	*bytes.Buffer
}

func (m *mockWriteCloser) Close() error {
	return nil
}

func TestOSCommandExecutor_StringCommand(t *testing.T) {
	exec := NewOSCommandExecutor(&mockFileSystem{})
	
	// Test that it can parse and run a simple string command
	res, err := exec.Run(context.Background(), "echo 'hello world'", "", nil, false)
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
	res, err := exec.Run(context.Background(), "printf '%s %s' 'arg one' \"arg two\"", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to run quoted command: %v", err)
	}

	if res.Stdout != "arg one arg two" {
		t.Errorf("Expected 'arg one arg two', got %q", res.Stdout)
	}
}

func TestOSCommandExecutor_InternalLogPath(t *testing.T) {
	fs := &mockFileSystem{}
	exec := NewOSCommandExecutor(fs)
	sessionID := "test-session-123"
	ctx := domain.WithSessionID(context.Background(), sessionID)

	t.Run("Logging enabled builds path internally", func(t *testing.T) {
		fs.createdPaths = nil
		_, err := exec.Run(ctx, "echo 'hello'", "", nil, true)
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
func TestOSCommandExecutor_FallbackTimeout(t *testing.T) {
	fs := &mockFileSystem{}
	exec := NewOSCommandExecutor(fs)
	
	// Set a very short fallback timeout
	exec.DefaultTimeout = 100 * time.Millisecond
	
	// Use context without deadline
	ctx := context.Background()
	
	// Run a command that sleeps longer than the timeout
	start := time.Now()
	res, err := exec.Run(ctx, "sleep 10", "", nil, false)
	duration := time.Since(start)
	
	if err == nil {
		t.Fatalf("Expected timeout error, got nil. Result: %+v, Duration: %v", res, duration)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}
	// Verify it timed out roughly at 100ms
	if duration > 500*time.Millisecond {
		t.Errorf("Command took too long to fail: %v", duration)
	}
}

func TestStreamingCmd_ID(t *testing.T) {
	fs := &mockFileSystem{}
	exec := NewOSCommandExecutor(fs)
	
	sc, err := exec.RunStreaming(context.Background(), "echo test", "", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	defer sc.Wait()

	id := sc.ID()
	if id == "" {
		t.Error("Expected non-empty ID from StreamingCmd")
	}
	
	if !strings.Contains(sc.LogPath(), id) {
		t.Errorf("Expected LogPath %q to contain ID %q", sc.LogPath(), id)
	}
}
