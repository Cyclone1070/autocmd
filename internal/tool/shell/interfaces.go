package shell

import (
	"context"
	"time"

	"github.com/Cyclone1070/iav/internal/tool/service/executor"
)

type envFileOps interface {
	ReadFile(path string) ([]byte, error)
}

// pathResolver defines workspace path resolution operations.
type pathResolver interface {
	Abs(path string) (string, error)
}

// commandExecutor defines the interface for executing shell commands.
// Return type is concrete from executor package per architecture guidelines:
// "Types and errors live with their implementation package"
type commandExecutor interface {
	RunStreaming(ctx context.Context, cmd []string, dir string, env []string, timeout time.Duration) (*executor.StreamingCmd, error)
}
