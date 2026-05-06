package grep

import (
	"context"
	"os"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
)

// pathResolver defines workspace path resolution operations.
type pathResolver interface {
	ValidateAbs(path string) (string, error)
	DisplayPath(path string) string
	Root() string
}

// fileSystem defines the minimal filesystem interface needed by search tools.
type fileSystem interface {
	Stat(path string) (os.FileInfo, error)
	ReadFile(path string) ([]byte, error)
	Remove(path string) error
	Open(path string) (domain.File, error)
}

// commandExecutor defines the interface for executing search commands.
type commandExecutor interface {
	Run(ctx context.Context, command string, dir string, enableLogging bool) (*executor.Result, error)
}
