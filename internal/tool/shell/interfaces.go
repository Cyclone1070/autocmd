package shell

import (
	"context"
	"io"
	"time"

	"github.com/Cyclone1070/iav/internal/tool/service/executor"
)

type envFileOps interface {
	ReadFile(path string) ([]byte, error)
}

// pathResolver defines workspace path resolution operations.
type pathResolver interface {
	Abs(path string) (string, error)
	Rel(path string) (string, error)
}

// streamingCommand represents a running command with streaming output.
// This is the consumer-defined interface for mockability.
type streamingCommand interface {
	Output() io.Reader
	Wait() (*executor.Result, error)
}

// commandExecutor defines the interface for executing shell commands.
type commandExecutor interface {
	RunStreaming(ctx context.Context, cmd []string, dir string, env []string, timeout time.Duration) (streamingCommand, error)
}
