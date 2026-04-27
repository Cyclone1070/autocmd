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

type mockGater struct {
	gateFunc func([]string) ([]string, int)
}

func (m *mockGater) Gate(lines []string) ([]string, int) { return m.gateFunc(lines) }

func newTestToolRenderer(t *testing.T) *ToolRenderer {
	t.Helper()
	cfg := config.DefaultConfig().UI()
	cfg.SetShortToolBlock(false) // Default tests to full mode
	themeCfg := ThemeConfig{
		PrimaryColor:   ToAdaptiveColor(cfg.PrimaryColor()),
		SuccessColor:   ToAdaptiveColor(cfg.SuccessColor()),
		ErrorColor:     ToAdaptiveColor(cfg.ErrorColor()),
		MutedColor:     ToAdaptiveColor(cfg.MutedColor()),
		ShortToolBlock: cfg.ShortToolBlock(),
	}
	theme := NewTheme(themeCfg)
	// For testing, we inject a gater that uses the standard 12 lines
	return NewToolRenderer(theme, 80, NewToolOutputGater(12))
}

func TestToolRenderer_RenderStringDoesNotUseGater(t *testing.T) {
	theme := NewTheme(ThemeConfig{})
	g := &mockGater{gateFunc: func(lines []string) ([]string, int) {
		return append(lines, "_gated"), 0
	}}
	tr := NewToolRenderer(theme, 80, g)

	got := tr.RenderString(domain.NewStringDisplay("", "raw"), StatusSuccess, "", "✓")
	assert.Contains(t, got, "raw")
	assert.NotContains(t, got, "_gated", "RenderString must not pass body through gater")
}

func TestToolRenderer_RespectsGaterOnBashOutput(t *testing.T) {
	theme := NewTheme(ThemeConfig{})
	g := &mockGater{gateFunc: func(lines []string) ([]string, int) {
		return append(lines, "_gated"), 0
	}}
	tr := NewToolRenderer(theme, 80, g)

	got := tr.RenderBash(domain.BashDisplay{Description: "C", Command: "cmd"}, "stdout", StatusSuccess, "", "✓")
	assert.Contains(t, got, "_gated", "RenderBash should gate captured output")
}

func TestToolRenderer_RenderQuestionDoesNotUseGater(t *testing.T) {
	theme := NewTheme(ThemeConfig{})
	g := &mockGater{gateFunc: func(lines []string) ([]string, int) {
		return append(lines, "_gated"), 0
	}}
	tr := NewToolRenderer(theme, 80, g)

	d := qDisplaySingle()
	s := NewQuestionUIState(d)
	got := tr.RenderQuestion(d, s, StatusRunning, "", "✓")
	assert.NotContains(t, got, "_gated", "RenderQuestion must not pass body through gater")
}

func assertGolden(t *testing.T, name string, actual string) {
	t.Helper()
	goldenFile := filepath.Join("testdata", name+".golden")

	if os.Getenv("UPDATE_GOLDEN") == "true" {
		err := os.WriteFile(goldenFile, []byte(actual), 0o644)
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
	display := domain.NewStringDisplay("", "Reading massive_file.txt...")
	got := tr.RenderString(display, StatusRunning, "", "⣾")
	assertGolden(t, "RenderString_Running", got)
}

func TestRenderString_ErrorWrap(t *testing.T) {
	tr := newTestToolRenderer(t)
	display := domain.NewStringDisplay("", "Reading file")
	got := tr.RenderString(display, StatusError, "permission denied", "✗")
	assertGolden(t, "RenderString_Error_Wrap", got)
}

func TestRenderDiff_DiffBody_Alignment(t *testing.T) {
	tr := newTestToolRenderer(t)
	diff := domain.DiffDisplay{
		Description: "Aligning logic",
		Target:      "Edit align.go",
		Diff:        "\n-line1\n+line2",
	}
	got := tr.RenderDiff(diff, StatusRunning, "", "⣾")
	assertGolden(t, "RenderDiff_DiffBody_Alignment", got)
}

func TestRenderDiff_SuccessWithStats(t *testing.T) {
	tr := newTestToolRenderer(t)
	diff := domain.DiffDisplay{
		Description: "Updating stats",
		Target:      "Edit file.go",
		Added:       5,
		Removed:     2,
		Diff:        "-old\n+new",
	}
	got := tr.RenderDiff(diff, StatusSuccess, "", "✓")
	assertGolden(t, "RenderDiff_Success_WithStats", got)
}

func TestRenderDiff_Error(t *testing.T) {
	tr := newTestToolRenderer(t)
	diff := domain.DiffDisplay{
		Description: "Missing file",
		Target:      "Edit file.go",
	}
	got := tr.RenderDiff(diff, StatusError, "file not found", "✗")
	assertGolden(t, "RenderDiff_Error", got)
}

func TestRenderDiff_ThreePartLayout(t *testing.T) {
	tr := newTestToolRenderer(t)
	diff := domain.DiffDisplay{
		Description: "Adding authentication middleware",
		Target:      "Edit auth.go",
		Added:       10,
		Removed:     5,
		Diff:        "+ new auth logic\n- old auth logic",
	}
	got := tr.RenderDiff(diff, StatusSuccess, "", "✓")
	assertGolden(t, "RenderDiff_ThreePartLayout", got)
}

func TestRenderBash_Running_Command(t *testing.T) {
	tr := newTestToolRenderer(t)
	display := domain.BashDisplay{
		Description: "List Files",
		Command:     "ls -la",
	}
	got := tr.RenderBash(display, "file1.txt\nfile2.txt", StatusRunning, "", "⣾")
	assertGolden(t, "RenderBash_Running_Command", got)
}

func TestRenderBash_LongOutputTruncation(t *testing.T) {
	tr := newTestToolRenderer(t)
	display := domain.BashDisplay{
		Description: "Log",
		Command:     "cat log.txt",
	}
	longOutput := strings.Repeat("line\n", 15)
	got := tr.RenderBash(display, longOutput, StatusSuccess, "", "✓")
	assertGolden(t, "RenderBash_Long_Output_Truncation", got)
}

func TestRenderBash_TruncatesByVisualLinesAfterWrap(t *testing.T) {
	theme := NewTheme(ThemeConfig{})
	tr := NewToolRenderer(theme, 30, NewToolOutputGater(2))
	display := domain.BashDisplay{
		Description: "Run",
		Command:     "echo x",
	}

	// One long logical line that wraps into multiple visual lines before truncation.
	got := tr.RenderBash(display, strings.Repeat("x", 100), StatusSuccess, "", "✓")

	assert.Contains(t, got, "▲ [", "wrapped overflow should be truncated by visual lines")
}

func TestRenderBash_TruncationIndicatorIsMutedInToolContent(t *testing.T) {
	cfg := config.DefaultConfig().UI()
	theme := NewTheme(ThemeConfig{
		PrimaryColor:   ToAdaptiveColor(cfg.PrimaryColor()),
		SuccessColor:   ToAdaptiveColor(cfg.SuccessColor()),
		ErrorColor:     ToAdaptiveColor(cfg.ErrorColor()),
		MutedColor:     ToAdaptiveColor(cfg.MutedColor()),
		ShortToolBlock: false,
	})
	tr := NewToolRenderer(theme, 80, NewToolOutputGater(2))
	display := domain.BashDisplay{
		Description: "Slow Shell",
		Command:     "slow-cmd",
	}

	got := tr.RenderBash(display, strings.Repeat("line\n", 5), StatusSuccess, "", "✓")
	expectedMutedIndicator := theme.Muted("  ▲ [3 lines truncated]")

	assert.Contains(t, got, expectedMutedIndicator, "truncate indicator should be muted inside tool content")
}

func TestRenderBash_Error(t *testing.T) {
	tr := newTestToolRenderer(t)
	display := domain.BashDisplay{
		Description: "List Files",
		Command:     "ls -la",
	}
	got := tr.RenderBash(display, "", StatusError, "exit status 1", "✗")
	assertGolden(t, "RenderBash_Error", got)
}

func TestRenderBash_ShowsCwdWhenPresent(t *testing.T) {
	tr := newTestToolRenderer(t)
	display := domain.BashDisplay{
		Description: "List Files",
		Command:     "ls -la",
		Cwd:         "/workspace/project",
	}
	got := tr.RenderBash(display, "file1.txt", StatusSuccess, "", "✓")
	assert.Contains(t, got, "/workspace/project")
	assert.Contains(t, got, "$ ls -la")
	assert.NotContains(t, got, "cwd:")
}

func TestRenderDiff_LongDiffTruncation(t *testing.T) {
	theme := NewTheme(ThemeConfig{})
	tr := NewToolRenderer(theme, 80, NewToolOutputGater(2))
	diff := domain.DiffDisplay{
		Description: "Massive Change",
		Target:      "Edit big.go",
		Diff:        "line 1\nline 2\nline 3\nline 4\nline 5",
	}
	got := tr.RenderDiff(diff, StatusSuccess, "", "✓")
	assertGolden(t, "RenderDiff_Long_Diff_Truncation", got)
}

func TestRenderDiff_TruncatesByVisualLinesAfterWrap(t *testing.T) {
	theme := NewTheme(ThemeConfig{})
	tr := NewToolRenderer(theme, 30, NewToolOutputGater(2))
	diff := domain.DiffDisplay{
		Description: "Big",
		Target:      "Edit x.go",
		Diff:        "+" + strings.Repeat("x", 100),
	}

	got := tr.RenderDiff(diff, StatusSuccess, "", "✓")

	assert.Contains(t, got, "▲ [", "wrapped overflow should be truncated by visual lines")
}

func TestRenderDiff_ShortMode(t *testing.T) {
	tr := newTestToolRenderer(t)
	tr.SetShortToolBlock(true)
	diff := domain.DiffDisplay{
		Description: "Massive Change",
		Target:      "Edit big.go",
		Diff:        "line 1\nline 2\nline 3",
	}
	output := tr.RenderDiff(diff, StatusSuccess, "", "✔")
	assert.NotContains(t, output, "line 1")
	assert.Contains(t, output, "Massive Change")
	assert.Contains(t, output, "Edit big.go")
}

func TestRenderBash_ShortMode(t *testing.T) {
	tr := newTestToolRenderer(t)
	tr.SetShortToolBlock(true)
	display := domain.BashDisplay{
		Description: "List Files",
		Command:     "ls -la",
	}
	output := tr.RenderBash(display, "file1.txt\nfile2.txt", StatusSuccess, "", "✔")
	assert.NotContains(t, output, "file1.txt")
	assert.Contains(t, output, "List Files")
	assert.Contains(t, output, "ls -la")
}

func TestRenderString_UsesHeaderAndTailBlockShape(t *testing.T) {
	tr := newTestToolRenderer(t)
	display := domain.NewStringDisplay("Read \"main.go\"", "")

	got := tr.RenderString(display, StatusRunning, "", "")
	assert.Contains(t, got, "Read \"main.go\"")
	assert.NotContains(t, got, "╭")
	assert.NotContains(t, got, "╮")
}

func TestRenderQuestion_Single(t *testing.T) {
	tr := newTestToolRenderer(t)
	d := qDisplaySingle()
	s := NewQuestionUIState(d)
	got := tr.RenderQuestion(d, s, StatusRunning, "", "⣾")
	assertGolden(t, "RenderQuestion_Single", got)
}

func TestRenderQuestion_MultiQuestionActiveSecond(t *testing.T) {
	tr := newTestToolRenderer(t)
	d := qDisplayMultiTwoQuestions()
	s := NewQuestionUIState(d)
	s.Active = 1
	s.Per[1].Cursor = 0
	got := tr.RenderQuestion(d, s, StatusRunning, "", "⣾")
	assertGolden(t, "RenderQuestion_MultiQuestion", got)
}

func TestRenderQuestion_Review(t *testing.T) {
	tr := newTestToolRenderer(t)
	d := qDisplayMultiTwoQuestions()
	s := NewQuestionUIState(d)
	s.Per[0].MultiSelected[0] = true
	s.Active = 2
	s.ReviewCursor = 0
	got := tr.RenderQuestion(d, s, StatusRunning, "", "⣾")
	assertGolden(t, "RenderQuestion_Review", got)
}

func TestRenderQuestion_ReviewAllAnswered(t *testing.T) {
	tr := newTestToolRenderer(t)
	d := qDisplayMultiTwoQuestions()
	s := NewQuestionUIState(d)
	s.Per[0].MultiSelected[0] = true
	s.Per[1].SingleSelected = 0
	s.Active = 2
	s.ReviewCursor = 0
	got := tr.RenderQuestion(d, s, StatusRunning, "", "⣾")
	assertGolden(t, "RenderQuestion_ReviewAllAnswered", got)
}

func TestRenderQuestion_Error(t *testing.T) {
	tr := newTestToolRenderer(t)
	d := qDisplaySingle()
	s := NewQuestionUIState(d)
	got := tr.RenderQuestion(d, s, StatusError, "permission denied", "⣾")
	assertGolden(t, "RenderQuestion_Error", got)
}

func TestRenderQuestion_RunningDoesNotIncludeSpinnerPrefix(t *testing.T) {
	tr := newTestToolRenderer(t)
	d := qDisplaySingle()
	s := NewQuestionUIState(d)
	got := tr.RenderQuestion(d, s, StatusRunning, "", "⣾")
	assert.Contains(t, got, "⣾")
}

func TestRenderQuestion_CustomRowOnlyWhenVisible(t *testing.T) {
	tr := newTestToolRenderer(t)
	d := qDisplaySingle()
	s := NewQuestionUIState(d)

	got := tr.RenderQuestion(d, s, StatusRunning, "", "⣾")
	assert.NotContains(t, got, "Other")

	s.Per[0].Cursor = len(d.Questions[0].Options)
	s.Per[0].CustomInputFocused = true
	got = tr.RenderQuestion(d, s, StatusRunning, "", "⣾")
	assert.Contains(t, got, "3.")
	assert.Contains(t, got, "█")
	assert.NotContains(t, got, "Other")
}

func TestRenderQuestion_MultiCustomRowShowsCheckbox(t *testing.T) {
	tr := newTestToolRenderer(t)
	d := domain.NewQuestionDisplay([]domain.QuestionInfo{{
		Question: "Q", Options: []string{"A"}, MultiSelect: true,
	}})
	s := NewQuestionUIState(d)
	s.Per[0].CustomBuffer = "x"
	s.Per[0].Cursor = 1
	got := tr.RenderQuestion(d, s, StatusRunning, "", "⣾")
	assert.Contains(t, got, "[ ]")
	assert.Contains(t, got, "x")
}
