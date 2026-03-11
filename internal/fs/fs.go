package fs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/tool/helper/content"
)

// FileSystem defines the filesystem operations needed by the application.
type FileSystem interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
	WriteFileAtomic(path string, data []byte, perm os.FileMode) error
	MkdirAll(path string, perm os.FileMode) error
	EnsureDirs(path string) error
	UserHomeDir() (string, error)
	ListDir(path string) ([]os.DirEntry, error)
	Remove(path string) error
	Stat(path string) (os.FileInfo, error)
}

// OSFileSystem implements filesystem operations using the local OS filesystem primitives.
type OSFileSystem struct {
	maxFileSize int64
}

// NewOSFileSystem creates a new OSFileSystem.
func NewOSFileSystem(maxFileSize int64) *OSFileSystem {
	return &OSFileSystem{maxFileSize: maxFileSize}
}

// ReadFile reads the entire content of a file with safety limits.
// It checks for MaxFileSize and binary content.
func (fs *OSFileSystem) ReadFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	if fs.maxFileSize != -1 && info.Size() > fs.maxFileSize {
		return nil, fmt.Errorf("file %s exceeds max size (%d bytes)", path, fs.maxFileSize)
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	if content.IsBinaryContent(data) {
		return nil, fmt.Errorf("binary file: %s", path)
	}

	return data, nil
}

// WriteFileAtomic writes content to a file atomically using temp file + rename pattern.
// This ensures that if the process crashes mid-write, the original file remains intact.
// The temp file is created in the same directory as the target to ensure atomic rename.
func (fs *OSFileSystem) WriteFileAtomic(path string, content []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)

	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp in %s: %w", dir, err)
	}

	tmpPath := tmpFile.Name()
	needsCleanup := true

	defer func() {
		if tmpFile != nil {
			_ = tmpFile.Close()
		}
		if needsCleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(content); err != nil {
		return fmt.Errorf("write temp %s: %w", tmpPath, err)
	}

	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("sync temp %s: %w", tmpPath, err)
	}

	if err := tmpFile.Chmod(perm); err != nil {
		return fmt.Errorf("chmod temp %s: %w", tmpPath, err)
	}

	// Close file before rename (required on some systems)
	if err := tmpFile.Close(); err != nil {
		tmpFile = nil
		return fmt.Errorf("close temp %s: %w", tmpPath, err)
	}
	tmpFile = nil

	// Atomic rename is the critical operation that ensures consistency
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tmpPath, path, err)
	}
	needsCleanup = false

	return nil
}

// MkdirAll creates a directory and all necessary parents.
func (fs *OSFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// EnsureDirs is a legacy alias for MkdirAll on specific paths.
func (fs *OSFileSystem) EnsureDirs(path string) error {
	return fs.MkdirAll(path, domain.DefaultDirPerm)
}

// UserHomeDir returns the current user's home directory.
func (fs *OSFileSystem) UserHomeDir() (string, error) {
	return os.UserHomeDir()
}

// ListDir lists the contents of a directory.
// Returns a slice of DirEntry for each entry in the directory.
func (fs *OSFileSystem) ListDir(path string) ([]os.DirEntry, error) {
	return os.ReadDir(path)
}

// Stat returns the FileInfo for a file.
func (fs *OSFileSystem) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

// WriteFile writes the entire content to a file.
func (fs *OSFileSystem) WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(name, data, perm)
}

// Remove deletes a file.
func (fs *OSFileSystem) Remove(path string) error {
	return os.Remove(path)
}
