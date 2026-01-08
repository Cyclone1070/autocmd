package executor

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
)

func TestRun(t *testing.T) {
	cfg := config.DefaultConfig()
	exec := NewOSCommandExecutor(cfg)

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
		cfg := config.DefaultConfig()
		cfg.Tools.DefaultMaxCommandOutputSize = 10
		exec := NewOSCommandExecutor(cfg)

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

func TestRunWithTimeout(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Tools.DockerGracefulShutdownMs = 100
	exec := NewOSCommandExecutor(cfg)

	t.Run("CompletesBeforeTimeout", func(t *testing.T) {
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
		// sleep for 10 seconds, timeout in 100ms
		_, err := exec.RunWithTimeout(context.Background(), []string{"sleep", "10"}, "", nil, 100*time.Millisecond)
		if err != ErrTimeout {
			t.Errorf("expected ErrTimeout, got %v", err)
		}
	})

	t.Run("OutputCollectedOnTimeout", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Skipping timeout test on Windows")
		}
		// Write something then sleep
		cmd := []string{"sh", "-c", "echo starting; sleep 10"}
		res, err := exec.RunWithTimeout(context.Background(), cmd, "", nil, 500*time.Millisecond)
		if err != ErrTimeout {
			t.Errorf("expected ErrTimeout, got %v", err)
		}
		if strings.TrimSpace(res.Stdout) != "starting" {
			t.Errorf("expected stdout 'starting', got %q", res.Stdout)
		}
	})
}

func TestRunStreaming(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Tools.DockerGracefulShutdownMs = 100
	exec := NewOSCommandExecutor(cfg)

	t.Run("SimpleCommand", func(t *testing.T) {
		streamCmd, err := exec.RunStreaming(context.Background(), []string{"echo", "hello"}, "", nil, 5*time.Second)
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

	t.Run("EmptyCommand", func(t *testing.T) {
		_, err := exec.RunStreaming(context.Background(), []string{}, "", nil, 1*time.Second)
		if err != os.ErrInvalid {
			t.Errorf("expected os.ErrInvalid, got %v", err)
		}
	})

	t.Run("Timeout", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Skipping timeout test on Windows")
		}
		streamCmd, err := exec.RunStreaming(context.Background(), []string{"sleep", "10"}, "", nil, 100*time.Millisecond)
		if err != nil {
			t.Fatalf("unexpected error starting: %v", err)
		}

		// Drain output to prevent blocking
		go func() {
			buf := make([]byte, 1024)
			for {
				_, err := streamCmd.Output().Read(buf)
				if err != nil {
					break
				}
			}
		}()

		res, err := streamCmd.Wait()
		if err != ErrTimeout {
			t.Errorf("expected ErrTimeout, got %v", err)
		}
		if res.ExitCode != -1 {
			t.Errorf("expected exit code -1 for timeout, got %d", res.ExitCode)
		}
	})

	t.Run("MultipleWaitCalls", func(t *testing.T) {
		streamCmd, err := exec.RunStreaming(context.Background(), []string{"echo", "once"}, "", nil, 5*time.Second)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Drain output
		go func() {
			buf := make([]byte, 1024)
			for {
				_, err := streamCmd.Output().Read(buf)
				if err != nil {
					break
				}
			}
		}()

		// Call Wait multiple times
		res1, err1 := streamCmd.Wait()
		res2, err2 := streamCmd.Wait()
		res3, err3 := streamCmd.Wait()

		// All should return the same result
		if res1 != res2 || res2 != res3 {
			t.Error("multiple Wait() calls should return same result pointer")
		}
		if err1 != err2 || err2 != err3 {
			t.Error("multiple Wait() calls should return same error")
		}
		if !strings.Contains(res1.Stdout, "once") {
			t.Errorf("expected stdout to contain 'once', got %q", res1.Stdout)
		}
	})

	t.Run("TimeoutStartsAtCommandStart", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Skipping timeout test on Windows")
		}
		// Command runs for 200ms, timeout is 100ms
		// But we delay calling Wait() by 150ms
		// If timeout started at Wait(), the command would complete
		// If timeout started at command start, it should timeout
		streamCmd, err := exec.RunStreaming(context.Background(), []string{"sleep", "0.2"}, "", nil, 100*time.Millisecond)
		if err != nil {
			t.Fatalf("unexpected error starting: %v", err)
		}

		// Drain output
		go func() {
			buf := make([]byte, 1024)
			for {
				_, err := streamCmd.Output().Read(buf)
				if err != nil {
					break
				}
			}
		}()

		// Delay before calling Wait
		time.Sleep(50 * time.Millisecond)

		_, err = streamCmd.Wait()
		if err != ErrTimeout {
			t.Errorf("expected ErrTimeout (timeout should start at command start), got %v", err)
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
