package loop

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTextAnimator_EmptyEnqueue_NoOp(t *testing.T) {
	t.Parallel()
	a := NewTextAnimator(4)
	a.Enqueue("")
	assert.False(t, a.HasPending())
	_, ok := a.NextChunk()
	assert.False(t, ok)
}

func TestTextAnimator_SingleChunk_ReturnsWholeThenEmpty(t *testing.T) {
	t.Parallel()
	a := NewTextAnimator(4)
	a.Enqueue("Hi")
	assert.True(t, a.HasPending())

	chunk, ok := a.NextChunk()
	assert.True(t, ok)
	assert.Equal(t, "Hi", chunk)
	assert.False(t, a.HasPending())

	_, ok = a.NextChunk()
	assert.False(t, ok)
}

func TestTextAnimator_MultiChunk_ExactRuneCount(t *testing.T) {
	t.Parallel()
	a := NewTextAnimator(4)
	a.Enqueue("12345678")

	chunk, ok := a.NextChunk()
	assert.True(t, ok)
	assert.Equal(t, "1234", chunk)

	chunk, ok = a.NextChunk()
	assert.True(t, ok)
	assert.Equal(t, "5678", chunk)

	_, ok = a.NextChunk()
	assert.False(t, ok)
	assert.False(t, a.HasPending())
}

func TestTextAnimator_MultiByteRunes_NoSplit(t *testing.T) {
	t.Parallel()
	a := NewTextAnimator(4)
	// 4 CJK characters (each 3 bytes in UTF-8)
	a.Enqueue("日本語韓")

	chunk, ok := a.NextChunk()
	assert.True(t, ok)
	assert.Equal(t, "日本語韓", chunk)

	_, ok = a.NextChunk()
	assert.False(t, ok)
}

func TestTextAnimator_FlushAll_ReturnsAndClears(t *testing.T) {
	t.Parallel()
	a := NewTextAnimator(4)
	a.Enqueue("ABC")
	a.NextChunk() // consume "ABC", nothing left for 4 runes
	a.Enqueue("XYZ")

	got := a.FlushAll()
	assert.Equal(t, "XYZ", got)
	assert.False(t, a.HasPending())

	got = a.FlushAll()
	assert.Equal(t, "", got)
}

func TestTextAnimator_FlushAll_EmptyReturnsEmpty(t *testing.T) {
	t.Parallel()
	a := NewTextAnimator(4)
	got := a.FlushAll()
	assert.Equal(t, "", got)
}

func TestTextAnimator_HasPending_Accuracy(t *testing.T) {
	t.Parallel()
	a := NewTextAnimator(4)
	a.Enqueue("12")
	assert.True(t, a.HasPending())
	a.NextChunk()
	assert.False(t, a.HasPending())
	a.Enqueue("ABCD")
	assert.True(t, a.HasPending())
	a.NextChunk()
	assert.False(t, a.HasPending())
}

func TestTextAnimator_SequentialEnqueue_Accumulates(t *testing.T) {
	t.Parallel()
	a := NewTextAnimator(4)
	a.Enqueue("AB")
	a.Enqueue("CD")

	chunk, ok := a.NextChunk()
	assert.True(t, ok)
	assert.Equal(t, "ABCD", chunk)

	_, ok = a.NextChunk()
	assert.False(t, ok)
}
