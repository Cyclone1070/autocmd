package search

import (
	"context"
	"os"

	"github.com/Cyclone1070/iav/internal/tool/service/executor"
)

// pathResolver defines workspace path resolution operations.
type pathResolver interface {
	Abs(path string) (string, error)
	DisplayPath(path string) string
}

// fileSystem defines the minimal filesystem interface needed by search tools.
type fileSystem interface {
	Stat(path string) (os.FileInfo, error)
}

// commandExecutor defines the interface for executing search commands.
type commandExecutor interface {
	Run(ctx context.Context, cmd []string, dir string, env []string) (*executor.Result, error)
}
