package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test helpers (inlined)

func newTestTheme(t *testing.T) *theme {
	t.Helper()
	cfg := config.DefaultConfig()
	return newTheme(cfg.UI)
}

func assertGolden(t *testing.T, name string, actual string) {
	t.Helper()
	goldenFile := filepath.Join("testdata", name+".golden")

	if os.Getenv("UPDATE_GOLDEN") == "true" {
		err := os.WriteFile(goldenFile, []byte(actual), 0644)
		require.NoError(t, err, "failed to update golden file")
		return
	}

	expected, err := os.ReadFile(goldenFile)
	if os.IsNotExist(err) {
		if os.Getenv("UPDATE_GOLDEN") != "true" {
			t.Fatalf("golden file %s does not exist. Run with UPDATE_GOLDEN=true to generate.", goldenFile)
		}
		return
	}
	require.NoError(t, err, "failed to read golden file")
	assert.Equal(t, string(expected), actual, "snapshot mismatch for %s", name)
}

// RenderString tests

func TestRenderString_Running(t *testing.T) {
	theme := newTestTheme(t)
	display := domain.StringDisplay("Reading massive_file.txt...")
	output := renderString(theme, display, statusRunning, "", "⣾")
	assertGolden(t, "RenderString_Running", output)
}

func TestRenderString_ErrorWrap(t *testing.T) {
	theme := newTestTheme(t)
	display := domain.StringDisplay("Reading file")
	output := renderString(theme, display, statusError, "permission denied", "✗")
	assertGolden(t, "RenderString_Error_Wrap", output)
}

// RenderDiff tests

func TestRenderDiff_DiffBody_Alignment(t *testing.T) {
	theme := newTestTheme(t)
	diff := domain.DiffDisplay{
		Header: "align.go",
		Diff:   "\n-line1\n+line2",
	}
	output := renderDiff(60, theme, diff, statusRunning, "", "⣾")
	assertGolden(t, "RenderDiff_DiffBody_Alignment", output)
}

func TestRenderDiff_SuccessWithStats(t *testing.T) {
	theme := newTestTheme(t)
	diff := domain.DiffDisplay{
		Header:  "file.go",
		Added:   5,
		Removed: 2,
		Diff:    " @@ -1,2 +1,2 @@\n-old\n+new",
	}
	output := renderDiff(60, theme, diff, statusSuccess, "", "✓")
	assertGolden(t, "RenderDiff_Success_WithStats", output)
}

func TestRenderDiff_Error(t *testing.T) {
	theme := newTestTheme(t)
	diff := domain.DiffDisplay{
		Header: "file.go",
	}
	output := renderDiff(60, theme, diff, statusError, "file not found", "✗")
	assertGolden(t, "RenderDiff_Error", output)
}

// RenderShell tests

func TestRenderShell_Running_Command(t *testing.T) {
	theme := newTestTheme(t)
	display := domain.ShellDisplay{
		Header:  "List Files",
		Command: "ls -la",
	}
	output := renderShell(40, 12, theme, display, "file1.txt\nfile2.txt", statusRunning, "", "⣾")
	assertGolden(t, "RenderShell_Running_Command", output)
}

func TestRenderShell_LongOutputTruncation(t *testing.T) {
	theme := newTestTheme(t)
	display := domain.ShellDisplay{
		Header:  "Log",
		Command: "cat log.txt",
	}
	longOutput := strings.Repeat("line\n", 15)
	output := renderShell(40, 12, theme, display, longOutput, statusSuccess, "", "✓")
	assertGolden(t, "RenderShell_Long_Output_Truncation", output)
}

func TestRenderShell_Error(t *testing.T) {
	theme := newTestTheme(t)
	display := domain.ShellDisplay{
		Header:  "List Files",
		Command: "ls -la",
	}
	output := renderShell(40, 12, theme, display, "", statusError, "exit status 1", "✗")
	assertGolden(t, "RenderShell_Error", output)
}

// Pad tests

func TestPad_WithPrefix(t *testing.T) {
	output := pad("Line1\nLine2", "->")
	assertGolden(t, "Pad_With_Prefix", output)
}

func TestPad_WithoutPrefix(t *testing.T) {
	output := pad("Line1\nLine2", "")
	assertGolden(t, "Pad_Without_Prefix", output)
}

func TestPad_EmptyInput(t *testing.T) {
	output := pad("", "->")
	assertGolden(t, "Pad_Empty_Input", output)
}
