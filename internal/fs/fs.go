package fs

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomic writes content to a file atomically using temp file + rename pattern.
// This ensures that if the process crashes mid-write, the original file remains intact.
// The temp file is created in the same directory as the target to ensure atomic rename.
func WriteFileAtomic(path string, content []byte, perm os.FileMode) error {
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

// EnsureDir creates parent directories recursively if they don't exist.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}
