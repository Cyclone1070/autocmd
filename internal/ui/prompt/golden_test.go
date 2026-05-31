package prompt

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cyclone1070/autocmd/internal/config"
	"github.com/Cyclone1070/autocmd/internal/domain"
	"github.com/Cyclone1070/autocmd/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
)

var update = flag.Bool("update", false, "update golden files")

type PromptElement struct {
	ID     string
	Desc   string
	Events []domain.UIUpdate
}

func getPromptElements() []PromptElement {
	tcID := "tc-ok"
	tcErrID := "tc-err"
	return []PromptElement{
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
					Display: domain.NewBashDisplay("Running Tests", "go test ./...", "", ""),
				},
				domain.ToolEndEvent{
					CallID:  tcID,
					Display: domain.NewBashDisplay("Running Tests", "go test ./...", "", ""),
				},
			},
		},
		{
			ID: "TOOL_ERR",
			Events: []domain.UIUpdate{
				domain.ToolStartEvent{
					CallID:  tcErrID,
					Display: domain.NewBashDisplay("Failing Command", "false", "", ""),
				},
				func() domain.ToolEndEvent {
					d := domain.NewBashDisplay("Failing Command", "false", "", "")
					d.Error = "Execution failed"
					return domain.ToolEndEvent{CallID: tcErrID, Display: d}
				}(),
			},
		},
		{
			ID: "THINK",
			Events: []domain.UIUpdate{
				domain.TextEvent{Text: "thought 1s", IsThought: true},
			},
		},
	}
}

func TestPrompt_GoldenCombinations(t *testing.T) {
	elements := getPromptElements()
	cfg := config.DefaultConfig().UI()
	width := 80
	cfg.SetChatWindowWidth(width)
	isDark := true
	renderer := ui.NewGlamourRenderer(width, isDark)

	var goldenOutput bytes.Buffer

	// 1. Singles
	for _, e := range elements {
		name := fmt.Sprintf("SINGLE_%s", e.ID)
		renderPromptToGolden(&goldenOutput, name, cfg, renderer, width, e)
	}

	// 2. Pairs
	for _, e1 := range elements {
		for _, e2 := range elements {
			name := fmt.Sprintf("PAIR_%s_%s", e1.ID, e2.ID)
			renderPromptToGolden(&goldenOutput, name, cfg, renderer, width, e1, e2)
		}
	}

	// 3. Triples
	for _, e1 := range elements {
		for _, e2 := range elements {
			for _, e3 := range elements {
				name := fmt.Sprintf("TRIPLE_%s_%s_%s", e1.ID, e2.ID, e3.ID)
				renderPromptToGolden(&goldenOutput, name, cfg, renderer, width, e1, e2, e3)
			}
		}
	}

	goldenPath := filepath.Join("testdata", "prompt_combos.golden")
	if *update {
		err := os.MkdirAll("testdata", 0o750)
		assert.NoError(t, err)
		err = os.WriteFile(goldenPath, goldenOutput.Bytes(), 0o600)
		assert.NoError(t, err)
		t.Logf("Updated golden file: %s", goldenPath)
	} else {
		// #nosec G304
		expected, err := os.ReadFile(goldenPath)
		if err != nil {
			t.Fatalf("Failed to read golden file: %v. Run with -update to create it.", err)
		}
		assert.Equal(t, string(expected), goldenOutput.String(), "Prompt UI regression detected! Rendered combinations do not match golden file.")
	}
}

type dummyBus struct {
	updates chan domain.UIUpdate
}

func (d dummyBus) UIUpdates() <-chan domain.UIUpdate {
	return d.updates
}
func (d dummyBus) SendAction(domain.Action) {}

func renderPromptToGolden(w *bytes.Buffer, name string, cfg config.UIConfig, renderer ui.Renderer, width int, elems ...PromptElement) {
	var signals []string
	theme := &ui.Theme{
		PrimaryCol: ui.ToAdaptiveColor(cfg.PrimaryColor()),
		SuccessCol: ui.ToAdaptiveColor(cfg.SuccessColor()),
		ErrorCol:   ui.ToAdaptiveColor(cfg.ErrorColor()),
		MutedCol:   ui.ToAdaptiveColor(cfg.MutedColor()),
		TextCol:    ui.ToAdaptiveColor(cfg.TextColor()),
	}
	s := NewStream(renderer)
	thinking := NewThinkingRenderer(theme, width, ui.NewToolOutputGater(5))
	tooling := ui.NewToolRenderer(theme, 80, ui.NewToolOutputGater(12))
	spinner := ui.NewSpinnerRenderer(lipgloss.NewStyle().Foreground(theme.PrimaryColor()))

	bus := dummyBus{updates: make(chan domain.UIUpdate, 100)}
	m := NewModel(bus, thinking, tooling, spinner, theme, s, ui.NewNoOpGater(), cfg.ChatWindowWidth(), WithFlush(func(content string) tea.Cmd {
		signals = append(signals, content)
		return nil
	}))

	for _, e := range elems {
		for _, ev := range e.Events {
			m.isPolling = true
			res, _ := m.Update(busEventMsg{event: ev})
			m = res.(*Model)
			for range 100 {
				if m.state == stateFlushing {
					res, _ = m.Update(flushDoneMsg{})
					m = res.(*Model)
					continue
				}
				break
			}
		}
	}

	m = m.DrainAnimationForTest()

	var trace strings.Builder
	for _, s := range signals {
		lines := strings.SplitSeq(s, "\n")
		for l := range lines {
			trace.WriteString(l + "\n")
		}
	}
	trace.WriteString(m.View())

	fmt.Fprintf(w, "\n=== START [%s] ===\n", name)
	w.WriteString(trace.String())
	fmt.Fprintf(w, "\n=== END [%s] ===\n", name)
}

func (m *Model) DrainAnimationForTest() *Model {
	return m
}
