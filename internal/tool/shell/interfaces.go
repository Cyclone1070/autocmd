package shell

import (
	"context"

	"github.com/Cyclone1070/iav/internal/tool/service/executor"
)

// pathResolver defines workspace path resolution operations.
type pathResolver interface {
	Root() string
}

// commandExecutor defines the interface for executing shell commands.
// Return type is concrete from executor package per architecture guidelines:
// "Types and errors live with their implementation package"
type commandExecutor interface {
	RunStreaming(ctx context.Context, cmd []string, dir string, env []string) (*executor.StreamingCmd, error)
}
