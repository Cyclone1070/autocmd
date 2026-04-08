package history

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
)

var update = flag.Bool("update", false, "update golden files")

type TestElement struct {
	ID       string
	Msg      *schema.Message
	Displays domain.ToolDisplays
	Desc     string
}

func getHistoryElements() []TestElement {
	captured := "output line 1\noutput line 2"
	return []TestElement{
		{
			ID: "TXT",
			Msg: &schema.Message{
				Role:    schema.Assistant,
				Content: "This is a paragraph of text.",
			},
		},
		{
			ID: "QUOTE",
			Msg: &schema.Message{
				Role:    schema.Assistant,
				Content: "> This is a blockquote.\n> It has multiple lines.",
			},
		},
		{
			ID: "LIST",
			Msg: &schema.Message{
				Role:    schema.Assistant,
				Content: "- Item 1\n- Item 2\n  - Nested Item",
			},
		},
		{
			ID: "CODE",
			Msg: &schema.Message{
				Role:    schema.Assistant,
				Content: "```go\nfunc hello() {\n\tfmt.Println(\"world\")\n}\n```",
			},
		},
		{ID: "H1", Msg: &schema.Message{Role: schema.Assistant, Content: "# Header 1"}},
		{ID: "H2", Msg: &schema.Message{Role: schema.Assistant, Content: "## Header 2"}},
		{ID: "H3", Msg: &schema.Message{Role: schema.Assistant, Content: "### Header 3"}},
		{ID: "H4", Msg: &schema.Message{Role: schema.Assistant, Content: "#### Header 4"}},
		{ID: "H5", Msg: &schema.Message{Role: schema.Assistant, Content: "##### Header 5"}},
		{ID: "H6", Msg: &schema.Message{Role: schema.Assistant, Content: "###### Header 6"}},
		{
			ID: "TOOL_OK",
			Msg: &schema.Message{
				Role: schema.Assistant,
				ToolCalls: []schema.ToolCall{
					{ID: "tc-ok", Function: schema.FunctionCall{Name: "bash"}},
				},
			},
			Displays: domain.ToolDisplays{
				"tc-ok": domain.BashDisplay{
					TypeField:      "bash",
					Comment:        "Running Tests",
					Command:        "go test ./...",
					CapturedOutput: captured,
				},
			},
		},
		{
			ID: "TOOL_ERR",
			Msg: &schema.Message{
				Role: schema.Assistant,
				ToolCalls: []schema.ToolCall{
					{ID: "tc-err", Function: schema.FunctionCall{Name: "bash"}},
				},
			},
			Displays: domain.ToolDisplays{
				"tc-err": domain.BashDisplay{
					TypeField: "bash",
					Comment:   "Failing Command",
					Command:   "false",
				},
			},
		},
		{
			ID: "THINK",
			Msg: &schema.Message{
				Role:    schema.Assistant,
				Content: "✔ Thought for 1s",
			},
		},
	}
}

func TestHistory_GoldenCombinations(t *testing.T) {
	elements := getHistoryElements()
	cfg := config.DefaultConfig().UI()
	themeCfg := ui.ThemeConfig{
		PrimaryColor: ui.ToAdaptiveColor(cfg.PrimaryColor()),
		SuccessColor: ui.ToAdaptiveColor(cfg.SuccessColor()),
		ErrorColor:   ui.ToAdaptiveColor(cfg.ErrorColor()),
		MutedColor:   ui.ToAdaptiveColor(cfg.MutedColor()),
		ShortToolbox: cfg.ShortToolbox(),
	}
	theme := ui.NewTheme(themeCfg)
	width := 80
	isDark := true
	renderer := ui.NewGlamourRenderer(width, isDark)

	var goldenOutput bytes.Buffer

	// 1. Singles
	for _, e := range elements {
		name := fmt.Sprintf("SINGLE_%s", e.ID)
		msgs, displays := createHistoryData(e)
		renderHistoryToGolden(&goldenOutput, name, msgs, displays, renderer, theme, width, isDark)
	}

	// 2. Pairs
	for _, e1 := range elements {
		for _, e2 := range elements {
			name := fmt.Sprintf("PAIR_%s_%s", e1.ID, e2.ID)
			msgs, displays := createHistoryData(e1, e2)
			renderHistoryToGolden(&goldenOutput, name, msgs, displays, renderer, theme, width, isDark)
		}
	}

	// 3. Triples
	for _, e1 := range elements {
		for _, e2 := range elements {
			for _, e3 := range elements {
				name := fmt.Sprintf("TRIPLE_%s_%s_%s", e1.ID, e2.ID, e3.ID)
				msgs, displays := createHistoryData(e1, e2, e3)
				renderHistoryToGolden(&goldenOutput, name, msgs, displays, renderer, theme, width, isDark)
			}
		}
	}

	goldenPath := filepath.Join("testdata", "history_combos.golden")
	if *update {
		err := os.MkdirAll("testdata", 0755)
		assert.NoError(t, err)
		err = os.WriteFile(goldenPath, goldenOutput.Bytes(), 0644)
		assert.NoError(t, err)
		t.Logf("Updated golden file: %s", goldenPath)
	} else {
		expected, err := os.ReadFile(goldenPath)
		if err != nil {
			t.Fatalf("Failed to read golden file: %v. Run with -update to create it.", err)
		}
		assert.Equal(t, string(expected), goldenOutput.String(), "History UI regression detected! Rendered combinations do not match golden file.")
	}
}

func createHistoryData(elems ...TestElement) ([]*schema.Message, domain.ToolDisplays) {
	var contents []string
	var calls []schema.ToolCall
	displays := make(domain.ToolDisplays)

	for _, e := range elems {
		if e.Msg.Role == schema.Assistant {
			if e.Msg.Content != "" {
				contents = append(contents, e.Msg.Content)
			}
			calls = append(calls, e.Msg.ToolCalls...)
		}
		for k, v := range e.Displays {
			displays[k] = v
		}
	}

	msg := &schema.Message{
		Role:      schema.Assistant,
		Content:   strings.Join(contents, "\n\n"),
		ToolCalls: calls,
	}
	return []*schema.Message{msg}, displays
}

func renderHistoryToGolden(w *bytes.Buffer, name string, msgs []*schema.Message, displays domain.ToolDisplays, renderer ui.Renderer, theme *ui.Theme, width int, isDark bool) {
	var sb strings.Builder
	am := msgs[0]
	if am.Role != schema.Assistant {
		// This golden test only renders assistant messages for now
		return
	}
	NewHistoryBuilder(renderer, theme, width).renderAssistantMessage(&sb, am, displays)

	w.WriteString(fmt.Sprintf("=== START [%s] ===\n", name))
	w.WriteString(sb.String())
	w.WriteString(fmt.Sprintf("\n=== END [%s] ===\n\n", name))
}
