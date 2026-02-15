package engine

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Transition processes a message and returns updated state and effects.
func Transition(state *State, msg Msg, deps Deps) (*State, []Effect) {
	effects := make([]Effect, 0, 8)

	switch m := msg.(type) {
	case MsgThinking:
		state.Thinking = true
		effects = append(effects, EffectScheduleTick())

	case MsgText:
		state.Thinking = false
		flushEffects := flushCompletedTools(state, deps)
		effects = append(effects, flushEffects...)

		flushedBlocks, err := deps.Markdown.Append(m.Text)
		if err != nil {
			effects = append(effects, EffectPrint(fmt.Sprintf("\nFatal: markdown rendering failed: %v", err)))
			effects = append(effects, EffectQuit())
			return state, effects
		}
		for _, block := range flushedBlocks {
			eff := enqueuePrintRaw(state, block)
			if eff != nil {
				effects = append(effects, eff)
			}
		}
		if len(effects) == len(flushEffects) {
			updateMaxAbsoluteHeight(state, deps)
		}
		return state, effects

	case MsgToolStart:
		state.Thinking = false
		flushEffects := flushCompletedTools(state, deps)
		effects = append(effects, flushEffects...)

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
			Status:  StatusRunning,
		}
		state.Tools = append(state.Tools, ts)
		effects = append(effects, EffectScheduleTick())
		updateMaxAbsoluteHeight(state, deps)

	case MsgToolStream:
		for _, ts := range state.Tools {
			if ts.CallID == m.CallID {
				ts.ShellOutput += m.Chunk
				break
			}
		}
		updateMaxAbsoluteHeight(state, deps)

	case MsgToolEnd:
		for _, ts := range state.Tools {
			if ts.CallID == m.CallID {
				if m.Error != "" {
					ts.Status = StatusError
					ts.Err = m.Error
				} else {
					ts.Status = StatusSuccess
				}
				break
			}
		}
		flushEffects := flushCompletedTools(state, deps)
		effects = append(effects, flushEffects...)
		if len(flushEffects) == 0 {
			updateMaxAbsoluteHeight(state, deps)
		}

	case MsgDone:
		state.RunState = StateDone
		doneEffects := handleDoneEvent(state, deps)
		effects = append(effects, doneEffects...)

	case MsgPrintFinished:
		if state.ContentBeingPrinted != "" {
			if state.ContentBeingPrintedRaw {
				lines := strings.Count(state.ContentBeingPrinted, "\n")
				if !strings.HasSuffix(state.ContentBeingPrinted, "\n") {
					lines++
				}
				state.TotalFlushedLines += lines
			} else {
				state.TotalFlushedLines += strings.Count(state.ContentBeingPrinted, "\n") + 1
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

	textFlush, err := deps.Markdown.RenderRemaining()
	if err != nil {
		effects = append(effects, EffectPrint(fmt.Sprintf("\nFatal: markdown flushing failed: %v", err)))
	} else if textFlush != "" {
		if e := enqueuePrintRaw(state, strings.TrimRight(textFlush, "\n")); e != nil {
			effects = append(effects, e)
		}
	}

	for _, ts := range state.Tools {
		output := deps.ToolRenderer.Render(ts, deps.Spinner)
		if e := enqueuePrint(state, output); e != nil {
			effects = append(effects, e)
		}
	}
	state.Tools = nil

	finalStatus := statusBar(state, deps)
	finalStatus = strings.TrimPrefix(finalStatus, "\n")
	if e := enqueuePrint(state, finalStatus); e != nil {
		effects = append(effects, e)
	}

	return effects
}

func flushCompletedTools(state *State, deps Deps) []Effect {
	var effects []Effect
	for len(state.Tools) > 0 && state.Tools[0].Status != StatusRunning {
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

func updateMaxAbsoluteHeight(state *State, deps Deps) {
	content := renderContent(state, deps)
	if content == "" {
		return
	}
	currentHeight := strings.Count(content, "\n")
	totalFootprint := state.TotalFlushedLines + currentHeight
	if totalFootprint > state.MaxAbsoluteHeight {
		state.MaxAbsoluteHeight = totalFootprint
	}
}

func renderContent(state *State, deps Deps) string {
	var parts []string
	if state.ContentBeingPrinted != "" {
		parts = append(parts, deps.Layout.TruncateWithIndicator(state.ContentBeingPrinted, state.Geometry.TermHeight))
	}
	for _, item := range state.PrintQueue {
		if item.Content != "" {
			parts = append(parts, deps.Layout.TruncateWithIndicator(item.Content, state.Geometry.TermHeight))
		}
	}
	if p := deps.Markdown.Pending(); p != "" {
		parts = append(parts, deps.Layout.TruncateWithIndicator(p, state.Geometry.TermHeight))
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
		status := "Generating"
		if state.Thinking {
			status = "Thinking"
		}
		spinnerView := ""
		if deps.Spinner != nil {
			spinnerView = deps.Spinner.SpinnerView()
		}
		left = spinnerView + themeFunc(status)
	}

	width := state.Geometry.Width
	// Measure visible width (ignoring ANSI escape codes from styled spinner)
	leftLen := lipgloss.Width(left)
	rightLen := lipgloss.Width(contextInfo)
	neededWidth := leftLen + 1 + rightLen
	if width < neededWidth {
		return "\n\n" + left + "\n" + contextInfo
	}
	gap := width - leftLen - rightLen
	return "\n\n" + left + strings.Repeat(" ", gap) + contextInfo
}
