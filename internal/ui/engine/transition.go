package engine

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/ui/theme"
	"github.com/charmbracelet/lipgloss"
)

// Transition processes a message and returns updated state and effects.
func Transition(state *State, msg Msg, deps Deps) (*State, []Effect) {
	effects := make([]Effect, 0, 8)

	switch m := msg.(type) {
	case MsgTick:
		state.IdleTicks++
		// Typing simulation: move N characters from TypingBuffer to Markdown.Append
		const charsPerTick = 4
		if state.TypingBuffer != "" {
			take := charsPerTick
			if len(state.TypingBuffer) < take {
				take = len(state.TypingBuffer)
			}
			chunk := state.TypingBuffer[:take]
			state.TypingBuffer = state.TypingBuffer[take:]

			flushedBlocks, err := deps.Markdown.Append(chunk)
			if err != nil {
				effects = append(effects, EffectPrint(fmt.Sprintf("\nFatal: markdown rendering failed: %v", err)))
				effects = append(effects, EffectQuit())
				return state, effects
			}
			for _, block := range flushedBlocks {
				if eff := enqueuePrintRaw(state, block); eff != nil {
					effects = append(effects, eff)
				}
			}
			state.IdleTicks = 0 // Reset idle ticks when actively typing
		}
		effects = append(effects, EffectScheduleTick())

	case MsgText:
		state.TypingBuffer += m.Text
		state.IdleTicks = 0
		effects = append(effects, EffectScheduleTick())

	case MsgToolStart:
		// Flush typing buffer first
		if state.TypingBuffer != "" {
			deps.Markdown.Append(state.TypingBuffer)
			state.TypingBuffer = ""
		}

		// Flush remaining markdown before starting tool
		textFlush, err := deps.Markdown.RenderRemaining()
		if err != nil {
			effects = append(effects, EffectPrint(fmt.Sprintf("\nFatal: markdown flushing failed: %v", err)))
			effects = append(effects, EffectQuit())
			return state, effects
		}
		if textFlush != "" {
			if eff := enqueuePrint(state, strings.TrimRight(textFlush, "\n")); eff != nil {
				effects = append(effects, eff)
			}
		}

		ts := &ToolState{
			CallID:  m.CallID,
			Display: m.Display,
			Status:  theme.StatusRunning,
		}
		state.Tools = append(state.Tools, ts)
		state.IdleTicks = 0
		effects = append(effects, EffectScheduleTick())

	case MsgToolStream:
		for _, ts := range state.Tools {
			if ts.CallID == m.CallID {
				ts.ShellOutput += m.Chunk
				break
			}
		}
		state.IdleTicks = 0
		effects = append(effects, EffectScheduleTick())

	case MsgToolEnd:
		for _, ts := range state.Tools {
			if ts.CallID == m.CallID {
				if m.Error != "" {
					ts.Status = theme.StatusError
					ts.Err = m.Error
				} else {
					ts.Status = theme.StatusSuccess
				}
				break
			}
		}
		flushEffects := flushCompletedTools(state, deps)
		effects = append(effects, flushEffects...)
		state.IdleTicks = 0
		effects = append(effects, EffectScheduleTick())

	case MsgDone:
		state.RunState = StateDone
		doneEffects := handleDoneEvent(state, deps)
		effects = append(effects, doneEffects...)

	case MsgPrintFinished:
		if state.ContentBeingPrinted != "" {
			state.TotalFlushedLines += strings.Count(state.ContentBeingPrinted, "\n")
			if state.ContentBeingPrintedRaw {
				if !strings.HasSuffix(state.ContentBeingPrinted, "\n") {
					state.TotalFlushedLines++
				}
			} else {
				state.TotalFlushedLines++
			}
		}
		state.IsPrinting = false
		state.ContentBeingPrinted = ""
		state.ContentBeingPrintedRaw = false
		nextEff := startNextPrint(state)
		if nextEff != nil {
			effects = append(effects, nextEff)
		} else if (state.RunState == StateDone || state.RunState == StateCancelled) &&
			len(state.PrintQueue) == 0 && !state.IsPrinting {
			effects = append(effects, EffectQuit())
		}

	case MsgCtrlC:
		state.RunState = StateCancelled
		doneEffects := handleDoneEvent(state, deps)
		effects = append(effects, doneEffects...)
	}

	return state, effects
}

func handleDoneEvent(state *State, deps Deps) []Effect {
	var effects []Effect

	// Flush typing buffer and markdown
	if state.TypingBuffer != "" {
		deps.Markdown.Append(state.TypingBuffer)
		state.TypingBuffer = ""
	}

	textFlush, err := deps.Markdown.RenderRemaining()
	if err != nil {
		effects = append(effects, EffectPrint(fmt.Sprintf("\nFatal: markdown flushing failed: %v", err)))
	} else if textFlush != "" {
		if e := enqueuePrintRaw(state, strings.TrimRight(textFlush, "\n")); e != nil {
			effects = append(effects, e)
		}
	}

	// Flush tools
	for _, ts := range state.Tools {
		output := deps.ToolRenderer.Render(ts, deps.Spinner)
		if e := enqueuePrint(state, output); e != nil {
			effects = append(effects, e)
		}
	}
	state.Tools = nil

	// Final status bar
	finalStatus := statusBar(state, deps)
	finalStatus = strings.TrimPrefix(finalStatus, "\n")
	if e := enqueuePrint(state, finalStatus); e != nil {
		effects = append(effects, e)
	}

	return effects
}

func flushCompletedTools(state *State, deps Deps) []Effect {
	var effects []Effect
	for len(state.Tools) > 0 && state.Tools[0].Status != theme.StatusRunning {
		tool := state.Tools[0]
		state.Tools = state.Tools[1:]
		output := deps.ToolRenderer.Render(tool, deps.Spinner)
		if e := enqueuePrint(state, output); e != nil {
			effects = append(effects, e)
		}
	}
	return effects
}

func enqueuePrint(state *State, content string) Effect {
	if content == "" {
		return nil
	}
	state.PrintQueue = append(state.PrintQueue, PrintItem{Content: content, Raw: false})
	return startNextPrint(state)
}

func enqueuePrintRaw(state *State, content string) Effect {
	if content == "" {
		return nil
	}
	state.PrintQueue = append(state.PrintQueue, PrintItem{Content: content, Raw: true})
	return startNextPrint(state)
}

func startNextPrint(state *State) Effect {
	if state.IsPrinting || len(state.PrintQueue) == 0 {
		return nil
	}
	item := state.PrintQueue[0]
	state.PrintQueue = state.PrintQueue[1:]
	state.ContentBeingPrinted = item.Content
	state.ContentBeingPrintedRaw = item.Raw
	state.IsPrinting = true
	if item.Raw {
		return EffectPrintRaw(item.Content)
	}
	return EffectPrint(item.Content)
}

func renderContent(state *State, deps Deps) string {
	var parts []string
	for _, item := range state.PrintQueue {
		if item.Content != "" {
			parts = append(parts, deps.Layout.TruncateWithIndicator(item.Content, state.TermSize.Height))
		}
	}
	if p := deps.Markdown.Pending(); p != "" {
		parts = append(parts, deps.Layout.TruncateWithIndicator(p, state.TermSize.Height))
	}
	for _, t := range state.Tools {
		parts = append(parts, deps.ToolRenderer.Render(t, deps.Spinner))
	}
	return strings.Join(parts, "\n")
}

func statusBar(state *State, deps Deps) string {
	var themeFunc func(string) string
	switch state.RunState {
	case StateDone:
		themeFunc = deps.Theme.Success
	case StateCancelled:
		themeFunc = deps.Theme.Error
	default:
		themeFunc = deps.Theme.Primary
	}
	contextInfo := themeFunc("Context: 42%")

	var left string
	switch state.RunState {
	case StateDone:
		left = fmt.Sprintf("%s %s", themeFunc("✓"), themeFunc("Done"))
	case StateCancelled:
		left = fmt.Sprintf("%s %s", themeFunc("✗"), themeFunc("Cancelled"))
	default:
		// Not shown during runtime anyway
		left = themeFunc("Running")
	}

	width := state.TermSize.Width
	leftLen := lipgloss.Width(left)
	rightLen := lipgloss.Width(contextInfo)
	neededWidth := leftLen + 1 + rightLen
	if width < neededWidth {
		return "\n" + left + "\n" + contextInfo
	}
	gap := width - leftLen - rightLen
	return "\n" + left + strings.Repeat(" ", gap) + contextInfo
}
