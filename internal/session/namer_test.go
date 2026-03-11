package session

import (
	"context"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
)

type mockLLM struct {
	domain.LLM
	streams []*mockStream
}

func (m *mockLLM) Stream(ctx context.Context, msgs domain.Messages, tools []domain.Declaration) (domain.Stream, error) {
	if len(m.streams) == 0 {
		return nil, nil
	}
	s := m.streams[0]
	m.streams = m.streams[1:]
	return s, nil
}

type mockStream struct {
	domain.Stream
	ctx    context.Context
	chunks []domain.StreamChunk
	index  int
}

func (m *mockStream) Next() bool {
	if m.ctx != nil {
		select {
		case <-m.ctx.Done():
			return false
		default:
		}
	}
	if m.index < len(m.chunks) {
		m.index++
		return true
	}
	return false
}

func (m *mockStream) Chunk() domain.StreamChunk {
	return m.chunks[m.index-1]
}

func (m *mockStream) Err() error {
	if m.ctx != nil {
		select {
		case <-m.ctx.Done():
			return m.ctx.Err()
		default:
		}
	}
	return nil
}

func TestGenerateName(t *testing.T) {
	ctx := context.Background()

	t.Run("Context cancellation falls back immediately", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)

		ms := &mockStream{
			ctx: cancelCtx,
			chunks: []domain.StreamChunk{
				domain.TextChunk{Text: "Partial "},
				domain.TextChunk{Text: "response"},
			},
		}
		m := &mockLLM{
			streams: []*mockStream{ms},
		}

		cancel() // Cancel BEFORE calling
		name, err := GenerateName(cancelCtx, m, "Cancelled message")

		assert.NoError(t, err)
		assert.Equal(t, "Cancelled message", name) // Should fallback on cancellation
	})

	t.Run("Success", func(t *testing.T) {
		m := &mockLLM{
			streams: []*mockStream{
				{
					chunks: []domain.StreamChunk{
						domain.TextChunk{Text: "Fixing "},
						domain.TextChunk{Text: "UI "},
						domain.TextChunk{Text: "Bugs"},
					},
				},
			},
		}

		name, err := GenerateName(ctx, m, "I have a bug in my UI")
		assert.NoError(t, err)
		assert.Equal(t, "Fixing UI Bugs", name)
	})

	t.Run("Use first message content", func(t *testing.T) {
		m := &mockLLM{
			streams: []*mockStream{
				{
					chunks: []domain.StreamChunk{
						domain.TextChunk{Text: "Summary of First"},
					},
				},
			},
		}

		name, err := GenerateName(ctx, m, "This is the first message")
		assert.NoError(t, err)
		assert.Equal(t, "Summary of First", name)
	})

	t.Run("Empty response fallback", func(t *testing.T) {
		m := &mockLLM{
			streams: []*mockStream{
				{chunks: []domain.StreamChunk{}},
			},
		}

		name, err := GenerateName(ctx, m, "Short message")
		assert.NoError(t, err)
		assert.Equal(t, "Short message", name)
	})

	t.Run("Fallback on error", func(t *testing.T) {
		m := &mockLLM{
			streams: nil, // Will trigger error in Stream
		}

		name, err := GenerateName(ctx, m, "Very long message that should be truncated when used as a fallback because it is too long for a session name")
		assert.NoError(t, err)
		assert.Equal(t, "Very long message that should be truncated when us...", name)
	})
}
