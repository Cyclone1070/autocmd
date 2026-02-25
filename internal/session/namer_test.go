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

func (m *mockLLM) Stream(ctx context.Context, msgs []domain.Message, tools []domain.Declaration) (domain.Stream, error) {
	if len(m.streams) == 0 {
		return nil, nil
	}
	s := m.streams[0]
	m.streams = m.streams[1:]
	return s, nil
}

type mockStream struct {
	domain.Stream
	chunks []domain.StreamChunk
	index  int
}

func (m *mockStream) Next() bool {
	if m.index < len(m.chunks) {
		m.index++
		return true
	}
	return false
}

func (m *mockStream) Chunk() domain.StreamChunk {
	return m.chunks[m.index-1]
}

func (m *mockStream) Err() error { return nil }

func TestGenerateName(t *testing.T) {
	ctx := context.Background()

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
