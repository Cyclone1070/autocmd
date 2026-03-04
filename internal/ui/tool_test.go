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

func newTestTheme(t *testing.T) *Theme {
	t.Helper()
	cfg := config.DefaultConfig()
	return NewTheme(cfg.UI)
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

func TestRenderString_Running(t *testing.T) {
	th := newTestTheme(t)
	display := domain.NewStringDisplay("Reading massive_file.txt...")
	output := RenderString(th, display, StatusRunning, "", "⣾")
	assertGolden(t, "RenderString_Running", output)
}

func TestRenderString_ErrorWrap(t *testing.T) {
	th := newTestTheme(t)
	display := domain.NewStringDisplay("Reading file")
	output := RenderString(th, display, StatusError, "permission denied", "✗")
	assertGolden(t, "RenderString_Error_Wrap", output)
}

func TestRenderDiff_DiffBody_Alignment(t *testing.T) {
	th := newTestTheme(t)
	diff := domain.DiffDisplay{
		Header: "align.go",
		Diff:   "\n-line1\n+line2",
	}
	output := RenderDiff(60, th, diff, StatusRunning, "", "⣾")
	assertGolden(t, "RenderDiff_DiffBody_Alignment", output)
}

func TestRenderDiff_SuccessWithStats(t *testing.T) {
	th := newTestTheme(t)
	diff := domain.DiffDisplay{
		Header:  "file.go",
		Added:   5,
		Removed: 2,
		Diff:    "-old\n+new",
	}
	output := RenderDiff(60, th, diff, StatusSuccess, "", "✓")
	assertGolden(t, "RenderDiff_Success_WithStats", output)
}

func TestRenderDiff_Error(t *testing.T) {
	th := newTestTheme(t)
	diff := domain.DiffDisplay{
		Header: "file.go",
	}
	output := RenderDiff(60, th, diff, StatusError, "file not found", "✗")
	assertGolden(t, "RenderDiff_Error", output)
}

func TestRenderShell_Running_Command(t *testing.T) {
	th := newTestTheme(t)
	display := domain.ShellDisplay{
		Header:  "List Files",
		Command: "ls -la",
	}
	output := RenderShell(40, 12, th, display, "file1.txt\nfile2.txt", StatusRunning, "", "⣾")
	assertGolden(t, "RenderShell_Running_Command", output)
}

func TestRenderShell_LongOutputTruncation(t *testing.T) {
	th := newTestTheme(t)
	display := domain.ShellDisplay{
		Header:  "Log",
		Command: "cat log.txt",
	}
	longOutput := strings.Repeat("line\n", 15)
	output := RenderShell(40, 12, th, display, longOutput, StatusSuccess, "", "✓")
	assertGolden(t, "RenderShell_Long_Output_Truncation", output)
}

func TestRenderShell_Error(t *testing.T) {
	th := newTestTheme(t)
	display := domain.ShellDisplay{
		Header:  "List Files",
		Command: "ls -la",
	}
	output := RenderShell(40, 12, th, display, "", StatusError, "exit status 1", "✗")
	assertGolden(t, "RenderShell_Error", output)
}

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
