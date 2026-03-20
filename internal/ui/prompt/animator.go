package prompt

import "unicode/utf8"

// TextAnimator splits text into fixed-size rune chunks for streaming animation.
type TextAnimator struct {
	pending      string
	runesPerTick int
}

// NewTextAnimator creates a new TextAnimator.
func NewTextAnimator(runesPerTick int) *TextAnimator {
	return &TextAnimator{runesPerTick: runesPerTick}
}

// Enqueue appends text to the internal pending buffer. Calling with "" is a no-op.
func (a *TextAnimator) Enqueue(text string) {
	if text == "" {
		return
	}
	a.pending += text
}

// NextChunk returns the next chunk of up to runesPerTick runes from pending and advances.
// Returns ("", false) when pending is empty. Rune-boundary safe.
func (a *TextAnimator) NextChunk() (string, bool) {
	if a.pending == "" {
		return "", false
	}
	count := 0
	byteOffset := 0
	for count < a.runesPerTick && byteOffset < len(a.pending) {
		_, size := utf8.DecodeRuneInString(a.pending[byteOffset:])
		byteOffset += size
		count++
	}
	chunk := a.pending[:byteOffset]
	a.pending = a.pending[byteOffset:]
	return chunk, true
}

// HasPending returns true if there is content remaining in the pending buffer.
func (a *TextAnimator) HasPending() bool {
	return len(a.pending) > 0
}

// FlushAll returns all pending content and clears the buffer.
func (a *TextAnimator) FlushAll() string {
	out := a.pending
	a.pending = ""
	return out
}

// Pending returns read-only access to the pending buffer (for test assertions).
func (a *TextAnimator) Pending() string {
	return a.pending
}
