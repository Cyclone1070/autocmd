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

func newTestToolRenderer(t *testing.T) *ToolRenderer {
	t.Helper()
	cfg := config.DefaultConfig().UI()
	cfg.SetShortToolbox(false) // Default tests to full mode
	themeCfg := ThemeConfig{
		PrimaryColor: ToAdaptiveColor(cfg.PrimaryColor()),
		SuccessColor: ToAdaptiveColor(cfg.SuccessColor()),
		ErrorColor:   ToAdaptiveColor(cfg.ErrorColor()),
		MutedColor:   ToAdaptiveColor(cfg.MutedColor()),
		ShortToolbox: cfg.ShortToolbox(),
	}
	theme := NewTheme(themeCfg)
	return NewToolRenderer(theme, 80)
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
	tr := newTestToolRenderer(t)
	display := domain.NewStringDisplay("Reading massive_file.txt...")
	got := tr.RenderString(display, StatusRunning, "", "⣾")
	assertGolden(t, "RenderString_Running", got)
}

func TestRenderString_ErrorWrap(t *testing.T) {
	tr := newTestToolRenderer(t)
	display := domain.NewStringDisplay("Reading file")
	got := tr.RenderString(display, StatusError, "permission denied", "✗")
	assertGolden(t, "RenderString_Error_Wrap", got)
}

func TestRenderDiff_DiffBody_Alignment(t *testing.T) {
	tr := newTestToolRenderer(t)
	diff := domain.DiffDisplay{
		Comment: "Aligning logic",
		Target:  "Edit align.go",
		Diff:    "\n-line1\n+line2",
	}
	got := tr.RenderDiff(diff, StatusRunning, "", "⣾")
	assertGolden(t, "RenderDiff_DiffBody_Alignment", got)
}

func TestRenderDiff_SuccessWithStats(t *testing.T) {
	tr := newTestToolRenderer(t)
	diff := domain.DiffDisplay{
		Comment: "Updating stats",
		Target:  "Edit file.go",
		Added:   5,
		Removed: 2,
		Diff:    "-old\n+new",
	}
	got := tr.RenderDiff(diff, StatusSuccess, "", "✓")
	assertGolden(t, "RenderDiff_Success_WithStats", got)
}

func TestRenderDiff_Error(t *testing.T) {
	tr := newTestToolRenderer(t)
	diff := domain.DiffDisplay{
		Comment: "Missing file",
		Target:  "Edit file.go",
	}
	got := tr.RenderDiff(diff, StatusError, "file not found", "✗")
	assertGolden(t, "RenderDiff_Error", got)
}

func TestRenderDiff_ThreePartLayout(t *testing.T) {
	tr := newTestToolRenderer(t)
	diff := domain.DiffDisplay{
		Comment: "Adding authentication middleware",
		Target:  "Edit auth.go",
		Added:   10,
		Removed: 5,
		Diff:    "+ new auth logic\n- old auth logic",
	}
	got := tr.RenderDiff(diff, StatusSuccess, "", "✓")
	assertGolden(t, "RenderDiff_ThreePartLayout", got)
}

func TestRenderShell_Running_Command(t *testing.T) {
	tr := newTestToolRenderer(t)
	display := domain.ShellDisplay{
		Comment: "List Files",
		Command: "ls -la",
	}
	got := tr.RenderShell(display, "file1.txt\nfile2.txt", StatusRunning, "", "⣾")
	assertGolden(t, "RenderShell_Running_Command", got)
}

func TestRenderShell_LongOutputTruncation(t *testing.T) {
	tr := newTestToolRenderer(t)
	display := domain.ShellDisplay{
		Comment: "Log",
		Command: "cat log.txt",
	}
	longOutput := strings.Repeat("line\n", 15)
	got := tr.RenderShell(display, longOutput, StatusSuccess, "", "✓")
	assertGolden(t, "RenderShell_Long_Output_Truncation", got)
}

func TestRenderShell_Error(t *testing.T) {
	tr := newTestToolRenderer(t)
	display := domain.ShellDisplay{
		Comment: "List Files",
		Command: "ls -la",
	}
	got := tr.RenderShell(display, "", StatusError, "exit status 1", "✗")
	assertGolden(t, "RenderShell_Error", got)
}

func TestRenderDiff_LongDiffTruncation(t *testing.T) {
	tr := newTestToolRenderer(t)
	tr.SetMaxLines(2) // Limit to 2 lines, should show truncation indicator
	diff := domain.DiffDisplay{
		Comment: "Massive Change",
		Target:  "Edit big.go",
		Diff:    "line 1\nline 2\nline 3\nline 4\nline 5",
	}
	got := tr.RenderDiff(diff, StatusSuccess, "", "✓")
	assertGolden(t, "RenderDiff_Long_Diff_Truncation", got)
}

func TestPad_WithPrefix(t *testing.T) {
	tr := newTestToolRenderer(t)
	output := tr.Pad("Line1\nLine2", "->")
	assertGolden(t, "Pad_With_Prefix", output)
}

func TestPad_WithoutPrefix(t *testing.T) {
	tr := newTestToolRenderer(t)
	output := tr.Pad("Line1\nLine2", "")
	assertGolden(t, "Pad_Without_Prefix", output)
}

func TestPad_EmptyInput(t *testing.T) {
	tr := newTestToolRenderer(t)
	output := tr.Pad("", "->")
	assertGolden(t, "Pad_Empty_Input", output)
}

func TestRenderDiff_ShortMode(t *testing.T) {
	tr := newTestToolRenderer(t)
	tr.SetShortToolbox(true)
	diff := domain.DiffDisplay{
		Comment: "Massive Change",
		Target:  "Edit big.go",
		Diff:    "line 1\nline 2\nline 3",
	}
	output := tr.RenderDiff(diff, StatusSuccess, "", "✔")
	assert.NotContains(t, output, "line 1")
	assert.Contains(t, output, "Massive Change")
	assert.Contains(t, output, "Edit big.go")
}

func TestRenderShell_ShortMode(t *testing.T) {
	tr := newTestToolRenderer(t)
	tr.SetShortToolbox(true)
	display := domain.ShellDisplay{
		Comment: "List Files",
		Command: "ls -la",
	}
	output := tr.RenderShell(display, "file1.txt\nfile2.txt", StatusSuccess, "", "✔")
	assert.NotContains(t, output, "file1.txt")
	assert.Contains(t, output, "List Files")
	assert.Contains(t, output, "ls -la")
}
