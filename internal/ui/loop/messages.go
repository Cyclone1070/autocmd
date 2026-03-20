package loop

// tickMsg signals an animation tick for text streaming.
type tickMsg struct{}

// flushDoneMsg signals that a side-effect (print) has finished.
type flushDoneMsg struct{}
