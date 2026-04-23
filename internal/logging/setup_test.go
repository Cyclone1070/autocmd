package logging

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanupLogs_RemovesExpiredFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	now := time.Now()

	old := filepath.Join(dir, "old.log")
	newer := filepath.Join(dir, "new.log")

	require.NoError(t, os.WriteFile(old, []byte("old"), 0o644))
	require.NoError(t, os.WriteFile(newer, []byte("new"), 0o644))
	require.NoError(t, os.Chtimes(old, now.Add(-30*24*time.Hour), now.Add(-30*24*time.Hour)))

	require.NoError(t, cleanupLogs(dir, now, 14*24*time.Hour, 100*1024*1024))

	_, err := os.Stat(old)
	assert.Error(t, err)
	_, err = os.Stat(newer)
	assert.NoError(t, err)
}

func TestRotateIfNeeded_RenamesWhenOverLimit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "error.log")

	limit := int64(10)
	require.NoError(t, os.WriteFile(logPath, []byte("0123456789abc"), 0o644))
	require.NoError(t, rotateIfNeeded(logPath, limit, 5))

	_, err := os.Stat(filepath.Join(dir, "error.log.1"))
	assert.NoError(t, err)
}

