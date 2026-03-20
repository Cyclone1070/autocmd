package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
)

// --- Mocks ---

type mockLLM struct {
	id            string
	displayName   string
	contextWindow int
	streams       []*mockStream
	streamErr     error
}

func (m *mockLLM) ID() string          { return m.id }
func (m *mockLLM) DisplayName() string { return m.displayName }
func (m *mockLLM) ContextWindow() int  { return m.contextWindow }

func (m *mockLLM) ComputeTokens(ctx context.Context, msgs domain.Messages) (int, error) {
	return 100, nil
}

func (m *mockLLM) Stream(ctx context.Context, msgs domain.Messages, tools []domain.Declaration) (domain.Stream, error) {
	if m.streamErr != nil && len(m.streams) == 0 {
		return nil, m.streamErr
	}
	if len(m.streams) == 0 {
		return nil, fmt.Errorf("no more streams")
	}
	s := m.streams[0]
	m.streams = m.streams[1:]
	return s, nil
}

type mockStream struct {
	chunks []domain.StreamChunk
	err    error
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

func (m *mockStream) Err() error {
	return m.err
}

type mockEventSender struct {
	events chan domain.UIUpdate
}

func (m *mockEventSender) SendUIUpdate(ev domain.UIUpdate) {
	if m.events != nil {
		m.events <- ev
	}
}

// Ensure mockEventSender implements local eventSender
var _ eventSender = (*mockEventSender)(nil)

func newMockEventSender(size int) *mockEventSender {
	return &mockEventSender{events: make(chan domain.UIUpdate, size)}
}

func newTestLoop(tools []domain.Tool, m domain.LLM, events eventSender) *Loop {
	registry := newMockToolRegistry(tools)
	return NewLoop(m, registry, 5, events)
}

// --- Tests ---

func TestRun_SingleTurn_TextOnly(t *testing.T) {
	ctx := context.Background()
	m := &mockLLM{
		id: "test",
		streams: []*mockStream{
			{chunks: []domain.StreamChunk{domain.TextChunk{Text: "Hello!"}}},
		},
	}

	session := &domain.Session{}
	sender := newMockEventSender(10)
	l := newTestLoop([]domain.Tool{}, m, sender)
	err := l.Run(ctx, session, "Hi")

	assert.NoError(t, err)
	assert.Equal(t, 2, len(session.Messages))

	assert.IsType(t, domain.ThinkingEvent{}, <-sender.events)
	assert.Equal(t, domain.TextEvent{Text: "Hello!"}, <-sender.events)
}

func TestRun_SingleToolCall(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
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
	sender := newMockEventSender(10)
	l := newTestLoop([]domain.Tool{mt}, m, sender)
	err := l.Run(ctx, session, "Weather?")

	assert.NoError(t, err)
	assert.Equal(t, 4, len(session.Messages))

	assert.IsType(t, domain.ThinkingEvent{}, <-sender.events)
	assert.IsType(t, domain.ToolStartEvent{}, <-sender.events)
	assert.IsType(t, domain.ToolEndEvent{}, <-sender.events)
	assert.IsType(t, domain.ThinkingEvent{}, <-sender.events)
	assert.Equal(t, domain.TextEvent{Text: "It's sunny!"}, <-sender.events)
}

func TestRun_ToolStreaming_Events(t *testing.T) {
	ctx := context.Background()
	m := &mockLLM{
		id: "test",
		streams: []*mockStream{
			{chunks: []domain.StreamChunk{
				domain.ToolCall{ID: "tc-stream", Name: "bash"},
			}},
		},
	}

	mt := &mockTool{
		name: "bash",
		prepare: func(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
			return &mockInvocation{
				display: domain.NewShellDisplay("Run Bash", "echo chunk", nil, nil),
				execute: func(ctx context.Context) (string, error) {
					return "done", nil
				},
			}, nil
		},
	}

	sender := newMockEventSender(10)
	l := newTestLoop([]domain.Tool{mt}, m, sender)
	_ = l.Run(ctx, &domain.Session{}, "run")

	// We don't verify the actual streaming here (that's in tool_executor_test),
	// but we verify that ToolStart and ToolEnd are sent through the interface.
	assert.IsType(t, domain.ThinkingEvent{}, <-sender.events)
	assert.IsType(t, domain.ToolStartEvent{}, <-sender.events)
	assert.IsType(t, domain.ToolEndEvent{}, <-sender.events)
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

	l := NewLoop(m, newMockToolRegistry([]domain.Tool{mt}), 3, nil)

	session := &domain.Session{}
	err := l.Run(context.Background(), session, "go")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max iterations (3) reached")
	lastMsg := session.Messages[len(session.Messages)-1].(domain.UserMessage)
	assert.Equal(t, "[Max iterations reached]", lastMsg.Content)
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
	lastMsg := session.Messages[len(session.Messages)-1].(domain.UserMessage)
	assert.Equal(t, "[Session cancelled by user]", lastMsg.Content)
}

func TestRun_ParallelToolCalls(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	sender := newMockEventSender(20)

	m := &mockLLM{
		id: "test",
		streams: []*mockStream{
			{chunks: []domain.StreamChunk{
				domain.ToolCall{ID: "tc-1", Name: "t1"},
				domain.ToolCall{ID: "tc-2", Name: "t2"},
			}},
			{chunks: []domain.StreamChunk{domain.TextChunk{Text: "Done."}}},
		},
	}

	// Channels to coordinate deterministic parallel execution
	t1Started := make(chan struct{})
	t2Started := make(chan struct{})
	canFinish := make(chan struct{})

	mt1 := &mockTool{
		name: "t1",
		prepare: func(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
			return &mockInvocation{
				execute: func(ctx context.Context) (string, error) {
					close(t1Started)
					<-canFinish // Wait until test says we can finish
					return "R1", nil
				},
			}, nil
		},
	}
	mt2 := &mockTool{
		name: "t2",
		prepare: func(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
			return &mockInvocation{
				execute: func(ctx context.Context) (string, error) {
					close(t2Started)
					<-canFinish // Wait until test says we can finish
					return "R2", nil
				},
			}, nil
		},
	}

	l := newTestLoop([]domain.Tool{mt1, mt2}, m, sender)
	session := &domain.Session{}

	// Run in a separate goroutine so we can coordinate
	runDone := make(chan error, 1)
	go func() {
		runDone <- l.Run(ctx, session, "run")
	}()

	// Wait for BOTH to have started. If they were serial, we would deadlock here
	// because mt1 would be waiting on canFinish before it ever reached the code to start mt2.
	select {
	case <-t1Started:
		select {
		case <-t2Started:
			// Success: both started
		case <-time.After(500 * time.Millisecond):
			t.Fatal("t2 did not start in parallel with t1")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("t1 did not start")
	}

	// Now let them finish
	close(canFinish)

	err := <-runDone
	assert.NoError(t, err)

	// Verify order in session messages (TC order is preserved, but execution was parallel)
	m2 := session.Messages[2].(domain.ToolMessage)
	m3 := session.Messages[3].(domain.ToolMessage)
	assert.Equal(t, "tc-1", m2.ToolCallID)
	assert.Equal(t, "tc-2", m3.ToolCallID)
}

func TestRun_ParallelToolCalls_Cancelled_RecordsAll(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	m := &mockLLM{
		id: "test",
		streams: []*mockStream{
			{chunks: []domain.StreamChunk{
				domain.ToolCall{ID: "tc-1", Name: "t1"},
				domain.ToolCall{ID: "tc-2", Name: "t2"},
			}},
		},
	}

	mt1 := &mockTool{
		name: "t1",
		prepare: func(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
			return &mockInvocation{
				execute: func(ctx context.Context) (string, error) {
					// Give enough time for the parallel executor to start both
					time.Sleep(50 * time.Millisecond)
					cancel()
					return "", ctx.Err()
				},
			}, nil
		},
	}
	mt2 := &mockTool{
		name: "t2",
		prepare: func(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
			return &mockInvocation{
				execute: func(ctx context.Context) (string, error) {
					// Should be cancelled by mt1
					<-ctx.Done()
					return "", ctx.Err()
				},
			}, nil
		},
	}

	l := newTestLoop([]domain.Tool{mt1, mt2}, m, nil)
	session := &domain.Session{}

	err := l.Run(ctx, session, "run")
	assert.ErrorIs(t, err, context.Canceled)

	// User message + Assistant (with 2 calls) + 2 Tool responses + User cancellation message
	// Wait, prompt.go appends a cancellation message in a defer.
	// We expect 5 messages:
	// 0: User input
	// 1: Assistant with tool calls
	// 2: Tool 1 response (cancelled)
	// 3: Tool 2 response (cancelled)
	// 4: [Session cancelled by user]
	assert.Equal(t, 5, len(session.Messages))
	m2 := session.Messages[2].(domain.ToolMessage)
	m3 := session.Messages[3].(domain.ToolMessage)
	assert.Equal(t, "tc-1", m2.ToolCallID)
	assert.True(t, m2.ToolError)
	assert.Equal(t, "tc-2", m3.ToolCallID)
	assert.True(t, m3.ToolError)
}
