package loop

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFlushCoordinator_EmptyFlush_ReturnsFalse(t *testing.T) {
	t.Parallel()
	f := newFlushCoordinator()
	content, ok := f.Flush()
	assert.False(t, ok)
	assert.Empty(t, content)
}

func TestFlushCoordinator_SingleSegment(t *testing.T) {
	t.Parallel()
	f := newFlushCoordinator()
	f.Enqueue("hello")
	content, ok := f.Flush()
	assert.True(t, ok)
	assert.Equal(t, "hello", content)
}

func TestFlushCoordinator_MultipleSegments_NoSeparator(t *testing.T) {
	t.Parallel()
	f := newFlushCoordinator()
	f.Enqueue("foo", "bar", "baz")
	content, ok := f.Flush()
	assert.True(t, ok)
	assert.Equal(t, "foobarbaz", content)
}

func TestFlushCoordinator_FlushClearsState(t *testing.T) {
	t.Parallel()
	f := newFlushCoordinator()
	f.Enqueue("once")
	_, ok := f.Flush()
	assert.True(t, ok)
	_, ok2 := f.Flush()
	assert.False(t, ok2)
}

func TestFlushCoordinator_IsEmpty_Transitions(t *testing.T) {
	t.Parallel()
	f := newFlushCoordinator()
	assert.True(t, f.IsEmpty())
	f.Enqueue("x")
	assert.False(t, f.IsEmpty())
	f.Flush()
	assert.True(t, f.IsEmpty())
}

func TestFlushCoordinator_EnqueueEmptyStrings_NoOp(t *testing.T) {
	t.Parallel()
	f := newFlushCoordinator()
	f.Enqueue("")
	f.Enqueue("", "")
	assert.True(t, f.IsEmpty())
	_, ok := f.Flush()
	assert.False(t, ok)
}

func TestFlushCoordinator_EnqueueMixedEmptyAndNonEmpty(t *testing.T) {
	t.Parallel()
	f := newFlushCoordinator()
	f.Enqueue("a", "", "b")
	content, ok := f.Flush()
	assert.True(t, ok)
	assert.Equal(t, "ab", content)
}
