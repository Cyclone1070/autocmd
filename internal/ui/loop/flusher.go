package loop

// flushCoordinator buffers rendered segments and flushes them as a single string.
type flushCoordinator struct {
	queue []string
}

func newFlushCoordinator() *flushCoordinator {
	return &flushCoordinator{}
}

// Enqueue appends non-empty segments to the internal queue.
// Calling with zero args or all-empty strings is a no-op.
func (f *flushCoordinator) Enqueue(segments ...string) {
	for _, s := range segments {
		if s != "" {
			f.queue = append(f.queue, s)
		}
	}
}

// Flush joins all queued segments with "", clears the queue, and returns (joined, true).
// Returns ("", false) if the queue is empty.
func (f *flushCoordinator) Flush() (string, bool) {
	if len(f.queue) == 0 {
		return "", false
	}
	var out string
	for _, s := range f.queue {
		out += s
	}
	f.queue = nil
	return out, true
}

// IsEmpty returns whether the queue has no segments.
func (f *flushCoordinator) IsEmpty() bool {
	return len(f.queue) == 0
}
