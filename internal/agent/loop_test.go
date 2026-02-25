package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
)

func newTestLoop(tools []domain.Tool, m domain.LLM, events chan domain.Event) *Loop {
	cfg := &config.Config{
		Tools: config.ToolsConfig{MaxIterations: 5},
	}
	registry := newMockToolRegistry(tools)
	return NewLoop(m, registry, cfg, events)
}

// --- Tests ---

func TestRun_SingleTurn_TextOnly(t *testing.T) {
	ctx := context.Background()
	events := make(chan domain.Event, 10)
	m := &mockLLM{
		id: "test",
		streams: []*mockStream{
			{chunks: []domain.StreamChunk{domain.TextChunk{Text: "Hello!"}}},
		},
	}

	session := &domain.Session{}
	l := newTestLoop([]domain.Tool{}, m, events)
	err := l.Run(ctx, session, "Hi")

	assert.NoError(t, err)
	assert.Equal(t, 2, len(session.Messages))
	assert.Equal(t, "Hi", session.Messages[0].Content)
	assert.Equal(t, "Hello!", session.Messages[1].Content)

	assert.IsType(t, domain.ThinkingEvent{}, <-events)
	assert.Equal(t, domain.TextEvent{Text: "Hello!"}, <-events)
	assert.IsType(t, domain.DoneEvent{}, <-events)
}

func TestRun_SingleToolCall(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events := make(chan domain.Event, 10)
	m := &mockLLM{
		id: "test",
		streams: []*mockStream{
			{chunks: []domain.StreamChunk{
				domain.ToolCall{ID: "tc-1", Name: "get_weather"},
			}},
			{chunks: []domain.StreamChunk{
				domain.TextChunk{Text: "It's sunny!"},
			}},
		},
	}

	mt := &mockTool{
		name: "get_weather",
		prepare: func(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
			return &mockInvocation{content: "Sunny"}, nil
		},
	}

	session := &domain.Session{}
	l := newTestLoop([]domain.Tool{mt}, m, events)
	err := l.Run(ctx, session, "Weather?")

	assert.NoError(t, err)
	assert.Equal(t, 4, len(session.Messages))

	assert.IsType(t, domain.ThinkingEvent{}, <-events)
	assert.IsType(t, domain.ToolStartEvent{}, <-events)
	assert.IsType(t, domain.ToolEndEvent{}, <-events)
	assert.IsType(t, domain.ThinkingEvent{}, <-events)
	assert.Equal(t, domain.TextEvent{Text: "It's sunny!"}, <-events)
	assert.IsType(t, domain.DoneEvent{}, <-events)
}

func TestRun_MaxIterationsExceeded(t *testing.T) {
	m := &mockLLM{
		id: "infinite",
		streams: []*mockStream{
			{chunks: []domain.StreamChunk{domain.ToolCall{ID: "tc-inf", Name: "infinite"}}},
			{chunks: []domain.StreamChunk{domain.ToolCall{ID: "tc-inf", Name: "infinite"}}},
			{chunks: []domain.StreamChunk{domain.ToolCall{ID: "tc-inf", Name: "infinite"}}},
		},
	}

	mt := &mockTool{
		name: "infinite",
		prepare: func(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
			return &mockInvocation{content: "kept going"}, nil
		},
	}

	cfg := &config.Config{Tools: config.ToolsConfig{MaxIterations: 3}}
	l := NewLoop(m, newMockToolRegistry([]domain.Tool{mt}), cfg, nil)

	session := &domain.Session{}
	err := l.Run(context.Background(), session, "go")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max iterations (3) reached")
	assert.Equal(t, "[Max iterations reached]", session.Messages[len(session.Messages)-1].Content)
}

func TestRun_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	m := &mockLLM{
		id: "test",
		streams: []*mockStream{
			{chunks: []domain.StreamChunk{domain.TextChunk{Text: "ok"}}},
		},
	}

	l := newTestLoop([]domain.Tool{}, m, nil)
	session := &domain.Session{}
	err := l.Run(ctx, session, "hi")

	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, "[Session cancelled by user]", session.Messages[len(session.Messages)-1].Content)
}
