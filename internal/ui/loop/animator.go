package loop

import "unicode/utf8"

// textAnimator splits text into fixed-size rune chunks for streaming animation.
type textAnimator struct {
	pending      string
	runesPerTick int
}

func newTextAnimator(runesPerTick int) *textAnimator {
	return &textAnimator{runesPerTick: runesPerTick}
}

// Enqueue appends text to the internal pending buffer. Calling with "" is a no-op.
func (a *textAnimator) Enqueue(text string) {
	if text == "" {
		return
	}
	a.pending += text
}

// NextChunk returns the next chunk of up to runesPerTick runes from pending and advances.
// Returns ("", false) when pending is empty. Rune-boundary safe.
func (a *textAnimator) NextChunk() (string, bool) {
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
func (a *textAnimator) HasPending() bool {
	return len(a.pending) > 0
}

// FlushAll returns the entirety of pending and clears it. Returns "" if nothing pending.
func (a *textAnimator) FlushAll() string {
	out := a.pending
	a.pending = ""
	return out
}

// Pending returns read-only access to the pending buffer (for test assertions).
func (a *textAnimator) Pending() string {
	return a.pending
}
