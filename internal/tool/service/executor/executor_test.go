package executor

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRun(t *testing.T) {
	exec := NewOSCommandExecutor()

	t.Run("SimpleCommand", func(t *testing.T) {
		res, err := exec.Run(context.Background(), []string{"echo", "hello"}, "", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(res.Stdout) != "hello" {
			t.Errorf("expected stdout 'hello', got %q", res.Stdout)
		}
		if res.ExitCode != 0 {
			t.Errorf("expected exit code 0, got %d", res.ExitCode)
		}
	})

	t.Run("EmptyCommand", func(t *testing.T) {
		_, err := exec.Run(context.Background(), []string{}, "", nil)
		if err != os.ErrInvalid {
			t.Errorf("expected os.ErrInvalid, got %v", err)
		}
	})

	t.Run("NonZeroExit", func(t *testing.T) {
		cmd := []string{"false"}
		if runtime.GOOS == "windows" {
			cmd = []string{"cmd", "/c", "exit 1"}
		}
		res, err := exec.Run(context.Background(), cmd, "", nil)
		if err != nil {
			t.Errorf("unexpected error for non-zero exit: %v", err)
		}
		if res.ExitCode != 1 {
			t.Errorf("expected exit code 1, got %d", res.ExitCode)
		}
	})

	t.Run("Stderr", func(t *testing.T) {
		// Use a script that writes to stderr
		cmd := []string{"sh", "-c", "echo error >&2"}
		if runtime.GOOS == "windows" {
			cmd = []string{"cmd", "/c", "echo error 1>&2"}
		}
		res, err := exec.Run(context.Background(), cmd, "", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(res.Stderr) != "error" {
			t.Errorf("expected stderr 'error', got %q", res.Stderr)
		}
	})

	t.Run("LargeOutput", func(t *testing.T) {
		exec := NewOSCommandExecutor()
		exec.maxOutputSize = 10

		res, err := exec.Run(context.Background(), []string{"echo", "123456789012345"}, "", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.Truncated {
			t.Error("expected output to be truncated")
		}
		if len(res.Stdout) > 10 {
			t.Errorf("expected stdout length <= 10, got %d", len(res.Stdout))
		}
	})
}

type MockClock struct {
	afterCh chan chan time.Time
}

func NewMockClock() *MockClock {
	return &MockClock{
		afterCh: make(chan chan time.Time, 10),
	}
}

func (m *MockClock) After(d time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	m.afterCh <- ch
	return ch
}

func (m *MockClock) Trigger(t *testing.T) {
	t.Helper()
	select {
	case ch := <-m.afterCh:
		ch <- time.Now()
	case <-time.After(1 * time.Second):
		t.Fatal("MockClock.Trigger timed out waiting for an After call")
	}
}

func TestRunWithTimeout(t *testing.T) {
	t.Run("CompletesBeforeTimeout", func(t *testing.T) {
		exec := NewOSCommandExecutor()
		res, err := exec.RunWithTimeout(context.Background(), []string{"echo", "hi"}, "", nil, 1*time.Second)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(res.Stdout) != "hi" {
			t.Errorf("expected stdout 'hi', got %q", res.Stdout)
		}
	})

	t.Run("TimeoutKillsProcess", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Skipping timeout test on Windows")
		}
		clock := NewMockClock()
		exec := NewOSCommandExecutor()
		exec.clock = clock

		errCh := make(chan error, 1)
		go func() {
			_, err := exec.RunWithTimeout(context.Background(), []string{"sleep", "10"}, "", nil, 1*time.Hour)
			errCh <- err
		}()

		// Trigger the main timeout
		clock.Trigger(t)
		// Trigger the graceful shutdown timeout (SIGKILL wait)
		clock.Trigger(t)

		err := <-errCh
		if err != ErrTimeout {
			t.Errorf("expected ErrTimeout, got %v", err)
		}
	})

	t.Run("OutputCollectedOnTimeout", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Skipping timeout test on Windows")
		}
		clock := NewMockClock()
		exec := NewOSCommandExecutor()
		exec.clock = clock

		resCh := make(chan *Result, 1)
		go func() {
			// Write something then sleep. We use a script to ensure some output is flushed.
			cmd := []string{"sh", "-c", "printf starting; sleep 10"}
			res, _ := exec.RunWithTimeout(context.Background(), cmd, "", nil, 1*time.Hour)
			resCh <- res
		}()

		// Give time for sh to start and printf to run
		time.Sleep(100 * time.Millisecond)

		clock.Trigger(t) // Main timeout
		clock.Trigger(t) // Graceful wait

		res := <-resCh
		if strings.TrimSpace(res.Stdout) != "starting" {
			t.Errorf("expected stdout 'starting', got %q", res.Stdout)
		}
	})
}

func TestRunStreaming(t *testing.T) {
	t.Run("SimpleCommand", func(t *testing.T) {
		exec := NewOSCommandExecutor()
		streamCmd, err := exec.RunStreaming(context.Background(), []string{"echo", "hello"}, "", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Read streaming output
		buf := make([]byte, 1024)
		n, _ := streamCmd.Output().Read(buf)
		streamOutput := string(buf[:n])

		// Wait for result
		res, err := streamCmd.Wait()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(streamOutput, "hello") {
			t.Errorf("expected stream to contain 'hello', got %q", streamOutput)
		}
		if !strings.Contains(res.Stdout, "hello") {
			t.Errorf("expected stdout to contain 'hello', got %q", res.Stdout)
		}
		if res.ExitCode != 0 {
			t.Errorf("expected exit code 0, got %d", res.ExitCode)
		}
	})
}

func TestCollector(t *testing.T) {
	t.Run("UnderLimit", func(t *testing.T) {
		c := newCollector(10, 5)
		n, err := c.Write([]byte("abc"))
		if err != nil || n != 3 {
			t.Errorf("unexpected write result: %v, %d", err, n)
		}
		if c.String() != "abc" || c.Truncated() {
			t.Errorf("unexpected collector state: %q, %v", c.String(), c.Truncated())
		}
	})

	t.Run("OverLimit", func(t *testing.T) {
		c := newCollector(5, 5)
		_, _ = c.Write([]byte("abcdef"))
		if c.String() != "abcde" || !c.Truncated() {
			t.Errorf("unexpected collector state: %q, %v", c.String(), c.Truncated())
		}
	})

	t.Run("BinaryDetection", func(t *testing.T) {
		c := newCollector(10, 5)
		_, _ = c.Write([]byte{'a', 0, 'b'})
		if c.String() != "[Binary Content]" || !c.Truncated() {
			t.Errorf("unexpected collector state: %q, %v", c.String(), c.Truncated())
		}
	})
}
