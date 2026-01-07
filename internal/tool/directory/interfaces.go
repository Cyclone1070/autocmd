package directory

import "os"

// pathResolver defines workspace path resolution operations.
type pathResolver interface {
	Abs(path string) (string, error)
	Rel(path string) (string, error)
}

// dirLister defines the filesystem operations needed for listing directories.
type dirLister interface {
	Stat(path string) (os.FileInfo, error)
	ListDir(path string) ([]os.FileInfo, error)
}

// ignoreMatcher defines the interface for gitignore pattern matching.
type ignoreMatcher interface {
	ShouldIgnore(relativePath string) bool
}
