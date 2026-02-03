package ui

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

// harness wraps the model for easier integration testing
type harness struct {
	m     tea.Model
	t     *testing.T
	model *model // Type-asserted pointer for direct access if needed
}

func newHarness(t *testing.T) *harness {
	cfg := config.DefaultConfig()
	// Fix terminal width/height for deterministic output
	cfg.UI.ChatWindowWidth = 80
	// We can't easily mock term.GetSize here without refactoring newModel,
	// but newModel uses defaultTerminalHeight=24 if GetSize fails (which it likely will in test).
	// Let's assume 80x24 for now.

	m, err := newModel(cfg)
	if err != nil {
		t.Fatalf("failed to create model: %v", err)
	}

	// Force a specific height on the model if possible, or verify default.
	// model.height is unexported. relying on defaultTerminalHeight (24).

	// Initialize
	m.Init()

	return &harness{
		m:     m,
		t:     t,
		model: m,
	}
}

func (h *harness) update(ev domain.Event) {
	// Wrap in internal msg type
	var teaMsg tea.Msg = msg{Event: ev}
	var cmd tea.Cmd

	h.m, cmd = h.m.Update(teaMsg)
	h.processCmds(cmd)
}

// processCmds handles internal commands like spinner ticks or prints
func (h *harness) processCmds(cmd tea.Cmd) {
	if cmd == nil {
		return
	}

	// Execute the command (this is tricky in a unit test without the BubbleTea runtime)
	// For integration tests, we might want to manually tick the spinner or handle prints?
	// The View() snapshot relies on state. Commands usually purely side-effects or producing new Msgs.
	// For strict snapshotting, we might ignore side-effects unless they loop back.

	// EXCEPT: simple commands usually return a Msg.
	msg := cmd()
	if msg != nil {
		// If it returns a message (like spinner tick), verify if we should feedback loop?
		// For deterministic golden tests, we might explicitly TICK instead of auto-looping.
		// So we ignore generated commands here.
	}
}

func (h *harness) snapshot(name string) {
	// Normalize output for Golden (strip timestamps or variable bits if any)
	output := h.m.View()
	assertGolden(h.t, name, output)
}

func TestIntegration_MarkdownStreaming(t *testing.T) {
	h := newHarness(t)

	// 1. Initial State
	h.snapshot("streaming_01_initial")

	// 2. Start Thinking
	h.update(domain.ThinkingEvent{})
	h.snapshot("streaming_02_thinking")

	// 3. Start Streaming Text
	// "Here is a "
	h.update(domain.TextEvent{Text: "Here is a "})
	h.snapshot("streaming_03_partial_text")

	// "**bold** statement." (Split across events to test buffering)
	h.update(domain.TextEvent{Text: "**bo"})
	h.snapshot("streaming_04_split_bold_1")
	h.update(domain.TextEvent{Text: "ld** statement.\n\n"})
	h.snapshot("streaming_05_split_bold_2_flushed")

	// 4. Code Block
	h.update(domain.TextEvent{Text: "```go\n"})
	h.snapshot("streaming_06_code_start")

	h.update(domain.TextEvent{Text: "func main() {\n"})
	h.snapshot("streaming_07_code_content")

	h.update(domain.TextEvent{Text: "}\n```\n\n"})
	h.snapshot("streaming_08_code_end")
}

func TestIntegration_ToolOrdering(t *testing.T) {
	h := newHarness(t)

	h.update(domain.TextEvent{Text: "Starting tools...\n\n"})

	// Start Tool A (Slow) - Should be index 0
	h.update(domain.ToolStartEvent{
		CallID:   "call_A",
		ToolName: "slow-tool",
		Display:  domain.StringDisplay("Tool A Running..."),
	})
	h.snapshot("tools_01_A_started")

	// Start Tool B (Fast) - Should be index 1
	h.update(domain.ToolStartEvent{
		CallID:   "call_B",
		ToolName: "fast-tool",
		Display:  domain.StringDisplay("Tool B Running..."),
	})
	h.snapshot("tools_02_B_started")

	// Finish Tool B (Success)
	// PROPER BEHAVIOR: Tool B is done, but A is still running.
	// Since A is first, A must display first. B is "finished" in state but still in queue behind A?
	// Implementation check: flushCompletedTools() checks `m.tools[0].status`.
	// If A[0] is Running, process stops. B[1] remains in list.
	h.update(domain.ToolEndEvent{CallID: "call_B"})
	h.snapshot("tools_03_B_finished_waiting_on_A")

	// Finish Tool A (Success)
	// PROPER BEHAVIOR: A finishes. Now flush A. Then B is at head -> flush B.
	h.update(domain.ToolEndEvent{CallID: "call_A"})
	h.snapshot("tools_04_A_finished_all_flushed")
}

func TestIntegration_DisplayTypes(t *testing.T) {
	h := newHarness(t)

	// 1. String Display (Error)
	h.update(domain.ToolStartEvent{
		CallID:   "call_str",
		ToolName: "string-tool",
		Display:  domain.StringDisplay("Simple text output"),
	})
	h.update(domain.ToolEndEvent{CallID: "call_str", Error: "Something went wrong"})
	h.snapshot("display_01_string_error")

	// 2. Diff Display
	h.update(domain.ToolStartEvent{
		CallID:   "call_diff",
		ToolName: "diff-tool",
		Display: domain.DiffDisplay{
			Header:  "diff.patch",
			Added:   2,
			Removed: 1,
			Diff:    "--- a\n+++ b\n-old\n+new\n+added",
		},
	})
	h.snapshot("display_02_diff_running")
	h.update(domain.ToolEndEvent{CallID: "call_diff"})
	h.snapshot("display_03_diff_success")

	// 3. Shell Display
	h.update(domain.ToolStartEvent{
		CallID:   "call_shell",
		ToolName: "shell-tool",
		Display: domain.ShellDisplay{
			Header:  "Build Project",
			Command: "make build",
		},
	})
	h.update(domain.ToolStreamEvent{CallID: "call_shell", Chunk: "Compiling...\n"})
	h.snapshot("display_04_shell_running")

	h.update(domain.ToolStreamEvent{CallID: "call_shell", Chunk: "Linking...\nDone.\n"})
	h.update(domain.ToolEndEvent{CallID: "call_shell"})
	h.snapshot("display_05_shell_success")
}

func TestIntegration_MarkdownStyling(t *testing.T) {
	h := newHarness(t)

	// Comprehensive Syntax Check
	text := `# Header 1
## Header 2
### Header 3

This is **bold**, _italic_, and ` + "`code`" + `.

> Blockquote line 1
> Blockquote line 2

- List item 1
- List item 2
  - Nested item

1. Ordered item 1
2. Ordered item 2

[Link Text](https://example.com)
`
	h.update(domain.TextEvent{Text: text})
	h.snapshot("styling_01_syntax_soup")
}

func TestIntegration_MarkdownSpacing(t *testing.T) {
	h := newHarness(t)

	// 1. Paragraph -> Paragraph
	h.update(domain.TextEvent{Text: "Paragraph One.\n\nParagraph Two.\n\n"})
	h.snapshot("spacing_01_para_to_para")

	// 2. Header -> Paragraph
	h.update(domain.TextEvent{Text: "# Header\n\nParagraph below header.\n\n"})
	h.snapshot("spacing_02_header_to_para")

	// 3. List -> Paragraph
	h.update(domain.TextEvent{Text: "- List Item\n\nParagraph below list.\n\n"})
	h.snapshot("spacing_03_list_to_para")

	// 4. Code Block -> Paragraph
	h.update(domain.TextEvent{Text: "```\ncode\n```\n\nParagraph below code.\n\n"})
	h.snapshot("spacing_04_code_to_para")
}
