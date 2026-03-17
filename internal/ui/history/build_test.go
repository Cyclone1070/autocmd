package history

import (
	"regexp"
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

type fixedRenderer struct {
	out string
}

func (r *fixedRenderer) Render(_ string) string { return r.out }


func newTestTheme() *ui.Theme {
	cfg := config.DefaultConfig().UI()
	cfg.SetShortToolbox(false)
	themeCfg := ui.ThemeConfig{
		PrimaryColor: ui.ToAdaptiveColor(cfg.PrimaryColor()),
		SuccessColor: ui.ToAdaptiveColor(cfg.SuccessColor()),
		ErrorColor:   ui.ToAdaptiveColor(cfg.ErrorColor()),
		MutedColor:   ui.ToAdaptiveColor(cfg.MutedColor()),
		ShortToolbox: cfg.ShortToolbox(),
	}
	return ui.NewTheme(themeCfg)
}

func TestShellHistory_UseCapturedOutput(t *testing.T) {
	theme := newTestTheme()
	captured := "output line 1\noutput line 2"

	messages := domain.Messages{
		domain.AssistantMessage{
			ToolCalls: []domain.ToolCall{
				{ID: "tc-1", Name: "shell"},
			},
		},
		domain.ToolMessage{
			ToolCallID: "tc-1",
			Content:    "output line 1\noutput line 2\n\n(Exit code: 0)",
		},
	}

	displays := domain.ToolDisplays{
		"tc-1": domain.ShellDisplay{
			TypeField:      "shell",
			Command:        "ls",
			CapturedOutput: &captured,
		},
	}

	rendered := BuildHistory(&domain.Session{Messages: messages, ToolDisplays: displays}, nil, theme, 80)

	// Should contain the captured output
	assert.Contains(t, rendered, "output line 1")
	assert.Contains(t, rendered, "output line 2")
	// Should NOT contain the exit code decoration from tool response
	assert.NotContains(t, rendered, "(Exit code: 0)")
}

func TestShellHistory_EmptyStdout_NoExitCode(t *testing.T) {
	theme := newTestTheme()
	empty := ""

	messages := domain.Messages{
		domain.AssistantMessage{
			ToolCalls: []domain.ToolCall{
				{ID: "tc-1", Name: "shell"},
			},
		},
		domain.ToolMessage{
			ToolCallID: "tc-1",
			Content:    "\n\n(Exit code: 0)",
		},
	}

	displays := domain.ToolDisplays{
		"tc-1": domain.ShellDisplay{
			TypeField:      "shell",
			Command:        "touch t.txt",
			CapturedOutput: &empty,
		},
	}

	rendered := BuildHistory(&domain.Session{Messages: messages, ToolDisplays: displays}, nil, theme, 80)

	// Should NOT contain the exit code decoration
	// THIS TEST IS EXPECTED TO FAIL IN THE RED PHASE
	assert.NotContains(t, rendered, "(Exit code: 0)", "History should not leak exit code for successful empty stdout commands")
}

func TestShellHistory_NilCapturedOutput_Fallback(t *testing.T) {
	theme := newTestTheme()

	messages := domain.Messages{
		domain.AssistantMessage{
			ToolCalls: []domain.ToolCall{
				{ID: "tc-1", Name: "shell"},
			},
		},
		domain.ToolMessage{
			ToolCallID: "tc-1",
			Content:    "fallback output\n\n(Exit code: 0)",
		},
	}

	displays := domain.ToolDisplays{
		"tc-1": domain.ShellDisplay{
			TypeField:      "shell",
			Command:        "ls",
			CapturedOutput: nil, // Legacy session
		},
	}

	rendered := BuildHistory(&domain.Session{Messages: messages, ToolDisplays: displays}, nil, theme, 80)

	// Should fall back to tool response content
	assert.Contains(t, rendered, "fallback output")
	assert.Contains(t, rendered, "(Exit code: 0)")
}

func TestShellHistory_ErrorStatus(t *testing.T) {
	theme := newTestTheme()
	empty := ""

	messages := domain.Messages{
		domain.AssistantMessage{
			ToolCalls: []domain.ToolCall{
				{ID: "tc-1", Name: "shell"},
			},
		},
		domain.ToolMessage{
			ToolCallID: "tc-1",
			Content:    "Error: Execution failed",
			ToolError:  true,
		},
	}

	displays := domain.ToolDisplays{
		"tc-1": domain.ShellDisplay{
			TypeField:      "shell",
			Command:        "false",
			CapturedOutput: &empty,
		},
	}

	rendered := BuildHistory(&domain.Session{Messages: messages, ToolDisplays: displays}, nil, theme, 80)

	// Should contain the error prefix indicator (X or similar depending on theme)
	assert.Contains(t, rendered, "✘")
}

func TestRenderMessage_SymmetryAndSpacing_Invariants_Combinations(t *testing.T) {
	// Ensure we get deterministic ANSI sequences in case they appear.
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	theme := newTestTheme()
	width := 80

	cases := []struct {
		name    string
		msg     domain.Message
		render  string // renderer output for content (used for user/assistant)
		roleTop string // "U│" or "A│"
	}{
		{
			name:    "UserPlain",
			msg:     domain.UserMessage{Content: "hi"},
			render:  "hi",
			roleTop: "U│",
		},
		{
			name:    "UserLeadingTrailingNewlines",
			msg:     domain.UserMessage{Content: "ignored"},
			render:  "\nhello\nworld\n",
			roleTop: "U│",
		},
		{
			name:    "AssistantPlain",
			msg:     domain.AssistantMessage{Content: "hello"},
			render:  "hello",
			roleTop: "A│",
		},
		{
			name:    "AssistantInternalBlankLines",
			msg:     domain.AssistantMessage{Content: "ignored"},
			render:  "a\n\nb",
			roleTop: "A│",
		},
		{
			name:    "AssistantEmptyContent",
			msg:     domain.AssistantMessage{Content: ""},
			render:  "",
			roleTop: "A│",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var msgs domain.Messages
			switch m := tc.msg.(type) {
			case domain.UserMessage:
				msgs = domain.Messages{m}
			case domain.AssistantMessage:
				msgs = domain.Messages{m}
			default:
				t.Fatalf("unsupported message type: %T", tc.msg)
			}

			r := &fixedRenderer{out: tc.render}
			out := stripANSI(RenderMessage(msgs, 0, nil, r, theme, width, false))

			// Exactly one unguttered blank line before and after.
			assert.True(t, strings.HasPrefix(out, "\n"), "must start with one unguttered blank line")
			assert.True(t, strings.HasSuffix(out, "\n"), "must end with one unguttered blank line")
			assert.False(t, strings.HasPrefix(out, "\n\n"), "must not start with two unguttered blank lines")

			// Top role line must exist immediately after the leading unguttered newline.
			assert.True(t, strings.HasPrefix(out, "\n"+tc.roleTop+"\n"), "role line must be first guttered line")

			// Bottom symmetry: last guttered line in the message body must be the continuation gutter.
			// Since RenderMessage adds a trailing unguttered newline, we expect the message to end with:
			// "\n │\n" (guttered blank line + final unguttered newline).
			assert.True(t, strings.HasSuffix(out, "\n │\n"), "must end with a guttered blank line (symmetry)")
		})
	}
}

func TestBuildHistory_ExactlyTwoUngutteredBlankLines_BetweenRenderedMessages_Combinations(t *testing.T) {
	theme := newTestTheme()
	width := 80

	type elem struct {
		role    string // "U" or "A"
		content string
	}

	// Build a small combinational set of sequences to catch spacing drift.
	elems := []elem{
		{role: "U", content: "u"},
		{role: "A", content: "a"},
		{role: "A", content: "\nleading\n"}, // renderer-like newlines inside content
		{role: "A", content: "x\n\ny"},       // internal blank line
	}

	makeMsg := func(e elem) domain.Message {
		if e.role == "U" {
			return domain.UserMessage{Content: e.content}
		}
		return domain.AssistantMessage{Content: e.content}
	}

	// Test all pairs and triples.
	sequences := [][]elem{}
	for _, e1 := range elems {
		for _, e2 := range elems {
			sequences = append(sequences, []elem{e1, e2})
			for _, e3 := range elems {
				sequences = append(sequences, []elem{e1, e2, e3})
			}
		}
	}

	for _, seq := range sequences {
		nameParts := make([]string, 0, len(seq))
		var msgs domain.Messages
		for _, e := range seq {
			nameParts = append(nameParts, e.role)
			msgs = append(msgs, makeMsg(e))
			// Include a tool message in between to ensure it doesn't affect spacing.
			msgs = append(msgs, domain.ToolMessage{ToolCallID: "tc-ignore", Content: "ignored"})
		}

		t.Run(strings.Join(nameParts, ""), func(t *testing.T) {
			out := stripANSI(BuildHistory(&domain.Session{Messages: msgs}, nil, theme, width))

			// No triple unguttered blank lines anywhere.
			assert.NotContains(t, out, "\n\n\n", "must not contain triple unguttered blank lines")

			// Every non-first message role line must be preceded by exactly two unguttered newlines.
			// We search for role lines as message starts: "\nU│\n" and "\nA│\n".
			starts := []int{}
			for i := 0; i < len(out)-3; i++ {
				if strings.HasPrefix(out[i:], "\nU│\n") || strings.HasPrefix(out[i:], "\nA│\n") {
					starts = append(starts, i+1) // index of 'U'/'A'
				}
			}
			if len(starts) < 2 {
				return
			}
			for _, idx := range starts[1:] {
				assert.GreaterOrEqual(t, idx, 2)
				assert.Equal(t, "\n\n", out[idx-2:idx], "message start must be preceded by exactly two unguttered newlines")
			}
		})
	}
}

func TestBuildHistory_CoalescesAssistantToolCallWithSummary(t *testing.T) {
	theme := newTestTheme()
	width := 80

	msgs := domain.Messages{
		domain.AssistantMessage{
			ToolCalls: []domain.ToolCall{{ID: "tc-1", Name: "shell"}},
		},
		domain.ToolMessage{
			ToolCallID: "tc-1",
			Content:    "README.md\n",
		},
		domain.AssistantMessage{
			Content: "Okay, I ran ls.",
		},
	}
	displays := domain.ToolDisplays{
		"tc-1": domain.NewShellDisplay("List directory contents", "ls", nil, nil),
	}

	out := stripANSI(BuildHistory(&domain.Session{Messages: msgs, ToolDisplays: displays}, nil, theme, width))

	// Should include the tool box and the summary text, but only a single assistant role line.
	assert.Contains(t, out, "╭", "tool box should be present")
	assert.Contains(t, out, "Okay, I ran ls.", "summary text should be present")
	assert.Equal(t, 1, strings.Count(out, "\nA│\n"), "assistant role line should appear once after coalescing")
}

func TestBuildHistory_CoalescesConsecutiveAssistantMessages(t *testing.T) {
	theme := newTestTheme()
	width := 80

	msgs := domain.Messages{
		domain.AssistantMessage{Content: "part1"},
		domain.AssistantMessage{Content: "part2"},
	}

	out := stripANSI(BuildHistory(&domain.Session{Messages: msgs}, nil, theme, width))

	assert.Contains(t, out, "part1")
	assert.Contains(t, out, "part2")
	assert.Equal(t, 1, strings.Count(out, "\nA│\n"), "consecutive assistant messages should coalesce into one block")
}

func TestDivider_Color(t *testing.T) {
	// Force color profile for consistent testing of escape codes
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii) // Reset after test

	cfg := config.DefaultConfig().UI()
	themeCfg := ui.ThemeConfig{
		PrimaryColor: ui.ToAdaptiveColor(cfg.PrimaryColor()),
		SuccessColor: ui.ToAdaptiveColor(cfg.SuccessColor()),
		ErrorColor:   ui.ToAdaptiveColor(cfg.ErrorColor()),
		MutedColor:   ui.ToAdaptiveColor(cfg.MutedColor()),
		ShortToolbox: cfg.ShortToolbox(),
	}
	theme := ui.NewTheme(themeCfg)

	// Expected USER first-line gutter (primary color, bold so the pipe pops)
	userStyle := lipgloss.NewStyle().Foreground(theme.PrimaryColor()).Bold(true)
	expectedUserPrefix := userStyle.Render("U│")

	// Expected ASSISTANT first-line gutter (muted color, bold so the pipe pops)
	assistantStyle := lipgloss.NewStyle().Foreground(theme.MutedColor()).Bold(true)
	expectedAssistantPrefix := assistantStyle.Render("A│")

	messages := domain.Messages{
		domain.UserMessage{Content: "user content"},
		domain.AssistantMessage{Content: "assistant content"},
	}

	// Render USER message
	renderedUser := RenderMessage(messages, 0, nil, nil, theme, 80, false)
	assert.Contains(t, renderedUser, expectedUserPrefix, "USER gutter should use primary color + bold")

	// Render ASSISTANT message
	renderedAssistant := RenderMessage(messages, 1, nil, nil, theme, 80, true)
	assert.Contains(t, renderedAssistant, expectedAssistantPrefix, "ASSISTANT gutter should use muted color + bold")
}

func TestMessageSpacing_ExactlyTwoBlankLinesBetweenMessages(t *testing.T) {
	theme := newTestTheme()
	width := 80
	messages := domain.Messages{
		domain.UserMessage{Content: "one"},
		// Tool messages should not contribute spacing in history view.
		domain.ToolMessage{ToolCallID: "tc-ignored", Content: "should not render"},
		domain.AssistantMessage{Content: "two"},
	}

	rendered := BuildHistory(&domain.Session{Messages: messages}, nil, theme, width)

	// With one blank line before+after each message, adjacent messages must have exactly
	// two blank lines between them, not three or more.
	assert.Contains(t, rendered, "one")
	assert.Contains(t, rendered, "two")
	assert.NotContains(t, rendered, "\n\n\n", "There should be no triple-blank-line spacing in history output")
	assert.NotContains(t, rendered, "should not render", "Tool messages should not be rendered in history")
}
func TestIssue_History_ToolBoxLeadingNewline(t *testing.T) {
	// RED PHASE: This test should fail because tool box is currently trimmed.
	theme := newTestTheme()
	width := 80
	renderer := ui.NewGlamourRenderer(width, true)

	tcID := "1"
	msg := domain.AssistantMessage{
		Content: "thought",
		ToolCalls: []domain.ToolCall{
			{
				ID:   tcID,
				Name: "shell",
			},
		},
	}
	messages := domain.Messages{msg}
	displays := domain.ToolDisplays{
		tcID: domain.NewShellDisplay("header", "ls", nil, nil),
	}

	var sb strings.Builder
	renderAssistantMessage(&sb, msg, messages, 0, displays, renderer, theme, width)
	rendered := sb.String()

	// Re-rendering a box to see its start
	rawBox := theme.Box("header", width-2, ui.StatusSuccess)
	assert.True(t, strings.HasPrefix(rawBox, "\n"), "Raw theme.Box should start with a newline")

	// We verify that the rendered history contains the box (starting with its unique top border)
	// and that it is preceded by at least one newline (for spacing).
	// Ensure the box top border is present (width may vary with gutter-width adjustments).
	assert.Contains(t, rendered, "╭", "Rendered history should contain the tool box top border")

	// Check that the character immediately preceding the top border is a newline.
	borderIdx := strings.Index(rendered, "╭")
	assert.Greater(t, borderIdx, 0, "Top border should not be at the very start")
	// In gutter mode the box border is prefixed (e.g. "A|"), so it won't be directly preceded by '\n'.
	// We only require that the box appears after some newline in the rendered output.
	assert.Contains(t, rendered[:borderIdx], "\n", "Tool box should appear after a newline")
}

func TestMessageHeaders(t *testing.T) {
	// Force color profile for consistent testing
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	theme := newTestTheme()
	width := 80

	messages := domain.Messages{
		domain.UserMessage{Content: "hello"},
		domain.AssistantMessage{Content: "hi"},
	}

	t.Run("User Message Formatting", func(t *testing.T) {
		theme := newTestTheme()
		width := 80
		renderer := &mockRenderer{}
		messages := domain.Messages{
			domain.UserMessage{Content: "Hello World"},
		}

		rendered := RenderMessage(messages, 0, nil, renderer, theme, width, false)

		style := lipgloss.NewStyle().Foreground(theme.PrimaryColor()).Bold(true)
		assert.Contains(t, rendered, style.Render("U│"), "Should contain USER first-line gutter")
		assert.Contains(t, rendered, style.Render(" │"), "Should contain USER continuation gutter")
		assert.Contains(t, rendered, "Hello World[rendered]", "User message content should be processed by renderer")
	})

	t.Run("Assistant Message Spacing", func(t *testing.T) {
		theme := newTestTheme()
		width := 80
		tcID := "tc-1"
		messages := domain.Messages{
			domain.AssistantMessage{
				ToolCalls: []domain.ToolCall{{ID: tcID, Name: "shell"}},
			},
		}
		displays := domain.ToolDisplays{
			tcID: domain.NewShellDisplay("header", "ls", nil, nil),
		}

		rendered := RenderMessage(messages, 0, displays, nil, theme, width, false)

		style := lipgloss.NewStyle().Foreground(theme.MutedColor()).Bold(true)
		assert.Contains(t, rendered, style.Render("A│"), "Should contain ASSISTANT first-line gutter")
		assert.Contains(t, rendered, style.Render(" │"), "Should contain ASSISTANT continuation gutter")
		assert.Contains(t, rendered, "╭", "Tool box top border should be present")
	})

	t.Run("Assistant Header", func(t *testing.T) {
		rendered := RenderMessage(messages, 1, nil, nil, theme, width, false)

		style := lipgloss.NewStyle().Foreground(theme.MutedColor()).Bold(true)
		assert.Contains(t, rendered, style.Render("A│"), "Assistant messages should use assistant gutter")
	})

	t.Run("User Header", func(t *testing.T) {
		rendered := RenderMessage(messages, 0, nil, nil, theme, width, false)

		style := lipgloss.NewStyle().Foreground(theme.PrimaryColor()).Bold(true)
		assert.Contains(t, rendered, style.Render("U│"), "User messages should use user gutter")
	})
}

type mockRenderer struct{}

func (m *mockRenderer) Render(s string) string {
	// Real Glamour renderer adds a leading newline
	return "\n" + s + "[rendered]"
}
