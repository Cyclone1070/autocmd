package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertGolden compares actual string with content of a golden file.
// If UPDATE_GOLDEN=true env var is set, it updates the golden file.
func assertGolden(t *testing.T, name string, actual string) {
	t.Helper()

	goldenFile := filepath.Join("testdata", name+".golden")

	if os.Getenv("UPDATE_GOLDEN") == "true" {
		err := os.WriteFile(goldenFile, []byte(actual), 0644)
		require.NoError(t, err, "failed to update golden file")
	}

	expected, err := os.ReadFile(goldenFile)
	if os.IsNotExist(err) {
		// New test case
		if os.Getenv("UPDATE_GOLDEN") != "true" {
			t.Fatalf("golden file %s does not exist. Run with UPDATE_GOLDEN=true to generate.", goldenFile)
		}
		// Pass if we just created it
		return
	}
	require.NoError(t, err, "failed to read golden file")

	assert.Equal(t, string(expected), actual, "snapshot mismatch for %s", name)
}
