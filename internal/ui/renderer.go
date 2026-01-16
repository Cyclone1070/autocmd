package ui

import (
	"io"

	"github.com/Cyclone1070/iav/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

// NewRenderer creates a new Renderer writing to the given output.
func NewRenderer(output io.Writer) *Renderer {
	m := newModel()
	p := tea.NewProgram(m, tea.WithOutput(output))
	return &Renderer{
		program: p,
		output:  output,
	}
}

// Send sends an event to the Bubble Tea program.
// It starts the program if it hasn't successfully run yet (though usually we start it explicitly).
// Actually, with Bubble Tea, we usually Start() the program and it runs the loop.
// Here we want to push events *into* the running program.
func (r *Renderer) Send(ev domain.Event) {
	// Program.Send is thread-safe
	r.program.Send(Msg{Event: ev})
}

// Start starts the UI. It returns immediately, running the program in a goroutine?
// No, tea.Program.Run() blocks.
// But we want to run it and feed it events from the workflow.
// So we should run it in a goroutine or have Wait() run it.
// The plan said:
// go func() { for ev := range events { renderer.Send(ev) } }()
// renderer.Wait()
//
// So Send() must work *before* Wait() is called? Or while Wait() is running?
// tea.Program.Send() works while Run() is active.
// So we should have a Start() method that returns detailed error or run it in Wait.

// Wait blocks until the program finishes.
// In our architecture:
// 1. main calls NewRenderer
// 2. main starts consuming events channel and calling Send()
// 3. main calls workflow.Run
// 4. main calls Wait()
//
// Issue: Program needs to be Run() for Send() to process messages.
// If we call Wait() at the end, who calls Run()?
//
// Correction: We should start the program immediately in a goroutine or have a Start() method.
// But tea.Run() takes over the input/output.
//
// Better approach for this integration:
// 1. NewRenderer creates program.
// 2. We expose a Run() method that blocks (wrapping program.Run).
// But workflow.Run() also blocks.
//
// We need to run the UI in the main goroutine (usually required for UI) and run workflow in a goroutine.
// OR run UI in a goroutine if it's just output (no input handling needed yet, but better be safe).
//
// However, the plan said:
// go func() { for ev := range events { renderer.Send(ev) } }()
// ...
// renderer.Wait()
//
// If renderer.Wait() calls program.Run(), we are good.
func (r *Renderer) Wait() error {
	_, err := r.program.Run()
	return err
}
