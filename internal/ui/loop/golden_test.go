package loop

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
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

var update = flag.Bool("update", false, "update golden files")

type LoopElement struct {
	ID     string
	Events []domain.UIUpdate
	Desc   string
}

func getLoopElements() []LoopElement {
	tcID := "tc-ok"
	tcErrID := "tc-err"
	return []LoopElement{
		{
			ID: "TXT",
			Events: []domain.UIUpdate{
				domain.TextEvent{Text: "This is a paragraph of text."},
			},
		},
		{
			ID: "QUOTE",
			Events: []domain.UIUpdate{
				domain.TextEvent{Text: "> This is a blockquote.\n> It has multiple lines."},
			},
		},
		{
			ID: "LIST",
			Events: []domain.UIUpdate{
				domain.TextEvent{Text: "- Item 1\n- Item 2\n  - Nested Item"},
			},
		},
		{
			ID: "CODE",
			Events: []domain.UIUpdate{
				domain.TextEvent{Text: "```go\nfunc hello() {\n\tfmt.Println(\"world\")\n}\n```"},
			},
		},
		{ID: "H1", Events: []domain.UIUpdate{domain.TextEvent{Text: "# Header 1"}}},
		{ID: "H2", Events: []domain.UIUpdate{domain.TextEvent{Text: "## Header 2"}}},
		{ID: "H3", Events: []domain.UIUpdate{domain.TextEvent{Text: "### Header 3"}}},
		{ID: "H4", Events: []domain.UIUpdate{domain.TextEvent{Text: "#### Header 4"}}},
		{ID: "H5", Events: []domain.UIUpdate{domain.TextEvent{Text: "##### Header 5"}}},
		{ID: "H6", Events: []domain.UIUpdate{domain.TextEvent{Text: "###### Header 6"}}},
		{
			ID: "TOOL_OK",
			Events: []domain.UIUpdate{
				domain.ToolStartEvent{
					CallID:  tcID,
					Display: domain.NewShellDisplay("Running Tests", "go test ./...", nil, nil),
				},
				domain.ToolEndEvent{
					CallID: tcID,
					Error:  "",
				},
			},
		},
		{
			ID: "TOOL_ERR",
			Events: []domain.UIUpdate{
				domain.ToolStartEvent{
					CallID:  tcErrID,
					Display: domain.NewShellDisplay("Failing Command", "false", nil, nil),
				},
				domain.ToolEndEvent{
					CallID: tcErrID,
					Error:  "Execution failed",
				},
			},
		},
		{
			ID: "THINK",
			Events: []domain.UIUpdate{
				domain.ThinkingEvent{},
				domain.TextEvent{Text: "thought 1s"}, // This finishes thinking in the loop logic
			},
		},
	}
}

func TestLoop_GoldenCombinations(t *testing.T) {
	elements := getLoopElements()
	cfg := config.DefaultConfig().UI
	width := 80
	cfg.ChatWindowWidth = width
	isDark := true
	renderer := ui.NewGlamourRenderer(width, isDark)

	var goldenOutput bytes.Buffer

	// 1. Singles
	for _, e := range elements {
		name := fmt.Sprintf("SINGLE_%s", e.ID)
		renderLoopToGolden(&goldenOutput, name, cfg, renderer, width, e)
	}

	// 2. Pairs
	for _, e1 := range elements {
		for _, e2 := range elements {
			name := fmt.Sprintf("PAIR_%s_%s", e1.ID, e2.ID)
			renderLoopToGolden(&goldenOutput, name, cfg, renderer, width, e1, e2)
		}
	}

	// 3. Triples
	for _, e1 := range elements {
		for _, e2 := range elements {
			for _, e3 := range elements {
				name := fmt.Sprintf("TRIPLE_%s_%s_%s", e1.ID, e2.ID, e3.ID)
				renderLoopToGolden(&goldenOutput, name, cfg, renderer, width, e1, e2, e3)
			}
		}
	}

	goldenPath := filepath.Join("testdata", "loop_combos.golden")
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
		assert.Equal(t, string(expected), goldenOutput.String(), "Loop UI regression detected! Rendered combinations do not match golden file.")
	}
}

type dummyBus struct{}

func (d dummyBus) UIUpdates() <-chan domain.UIUpdate { return nil }
func (d dummyBus) SendAction(domain.Action)           {}

func renderLoopToGolden(w *bytes.Buffer, name string, cfg config.UIConfig, renderer ui.Renderer, width int, elems ...LoopElement) {
	var signals []string
	m := NewModel(dummyBus{}, cfg, WithFlush(func(content string) tea.Cmd {
		signals = append(signals, content)
		return nil
	}))
	m.stream.renderer = renderer // Force our deterministic renderer

	for _, e := range elems {
		for _, ev := range e.Events {
			res, _ := m.Update(eventMsg{update: ev})
			m = res.(*Model)
		}
	}

	var trace strings.Builder
	for _, s := range signals {
		trace.WriteString(s)
	}
	trace.WriteString(m.View())

	w.WriteString(fmt.Sprintf("\n=== START [%s] ===\n", name))
	w.WriteString(trace.String())
	w.WriteString(fmt.Sprintf("\n=== END [%s] ===\n", name))
}
