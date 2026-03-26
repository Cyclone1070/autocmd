package session

import (
	"context"
	"fmt"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
)

type mockLLM struct {
	domain.LLM
	streams []*mockStream
}

func (m *mockLLM) ComputeTokens(ctx context.Context, msgs []*schema.Message) (int, error) {
	return 0, nil
}
func (m *mockLLM) Model() model.ToolCallingChatModel {
	return &mockEinoModelBridge{llm: m}
}

// mockEinoModelBridge adapts the old mockLLM.Stream for the new GenerateName
type mockEinoModelBridge struct {
	llm *mockLLM
}

func (b *mockEinoModelBridge) Generate(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(b.llm.streams) == 0 {
		return nil, fmt.Errorf("no more streams")
	}
	s := b.llm.streams[0]
	b.llm.streams = b.llm.streams[1:]

	var content string
	for _, chunk := range s.chunks {
		content += chunk.text
	}
	return &schema.Message{Role: schema.Assistant, Content: content}, nil
}

func (b *mockEinoModelBridge) Stream(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func (b *mockEinoModelBridge) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return b, nil
}

func (m *mockLLM) ID() string { return "test" }
type mockChunk struct {
	text string
}

type mockStream struct {
	ctx    context.Context
	chunks []mockChunk
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

func (m *mockStream) Chunk() mockChunk {
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
			chunks: []mockChunk{
				mockChunk{text: "Partial "},
				mockChunk{text: "response"},
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
					chunks: []mockChunk{
						mockChunk{text: "Fixing "},
						mockChunk{text: "UI "},
						mockChunk{text: "Bugs"},
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
					chunks: []mockChunk{
						mockChunk{text: "Summary of First"},
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
				{chunks: []mockChunk{}},
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
