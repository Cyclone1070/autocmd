package agent

import (
	"context"
	"fmt"

	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
)

func ptr[T any](v T) *T { return &v }

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

func (m *mockLLM) ComputeTokens(ctx context.Context, msgs []*schema.Message) (int, error) {
	return 100, nil
}

func (m *mockLLM) Model() model.ToolCallingChatModel {
	return &mockEinoModelBridge{llm: m}
}

// mockEinoModelBridge adapts the old mockLLM.Stream for the new loop.go
type mockEinoModelBridge struct {
	llm *mockLLM
}

func (b *mockEinoModelBridge) Generate(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return nil, nil
}

func (b *mockEinoModelBridge) Stream(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if b.llm.streamErr != nil && len(b.llm.streams) == 0 {
		return nil, b.llm.streamErr
	}
	if len(b.llm.streams) == 0 {
		return nil, fmt.Errorf("no more streams")
	}
	s := b.llm.streams[0]
	b.llm.streams = b.llm.streams[1:]

	sr, sw := schema.Pipe[*schema.Message](1)
	go func() {
		defer sw.Close()
		for _, chunk := range s.chunks {
			msg := &schema.Message{
				Role: schema.Assistant,
			}
			if chunk.toolCall != nil {
				msg.ToolCalls = []schema.ToolCall{*chunk.toolCall}
			} else if chunk.isThought {
				msg.ReasoningContent = chunk.text
			} else {
				msg.Content = chunk.text
			}
			sw.Send(msg, nil)
		}
		if s.err != nil {
			sw.Send(nil, s.err)
		}
	}()
	return sr, nil
}

func (b *mockEinoModelBridge) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return b, nil
}

type mockChunk struct {
	text      string
	isThought bool
	toolCall  *schema.ToolCall
}

type mockStream struct {
	chunks []mockChunk
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

func (m *mockStream) Chunk() mockChunk {
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


// mockToolRegistry is defined in tool_executor_test.go

// --- Tests ---

func TestRun_SingleTurn_TextOnly(t *testing.T) {
	ctx := context.Background()
	m := &mockLLM{
		id: "test",
		streams: []*mockStream{
			{chunks: []mockChunk{{text: "Hello!"}}},
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
			{chunks: []mockChunk{
				{toolCall: &schema.ToolCall{Index: ptr(0), ID: "tc-1", Function: schema.FunctionCall{Name: "get_weather"}}},
			}},
			{chunks: []mockChunk{
				{text: "It's sunny!"},
			}},
		},
	}

	mt := &mockTool{
		name: "get_weather",
		prepare: func(ctx context.Context, params string) (domain.Invocation, error) {
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
			{chunks: []mockChunk{
				{toolCall: &schema.ToolCall{Index: ptr(0), ID: "tc-stream", Function: schema.FunctionCall{Name: "bash"}}},
			}},
		},
	}

	mt := &mockTool{
		name: "bash",
		prepare: func(ctx context.Context, params string) (domain.Invocation, error) {
			return &mockInvocation{
				display: domain.NewShellDisplay("Run Bash", "echo chunk", ""),
				execute: func(ctx context.Context) (string, error) {
					return "done", nil
				},
			}, nil
		},
	}

	sender := newMockEventSender(10)
	l := newTestLoop([]domain.Tool{mt}, m, sender)
	_ = l.Run(ctx, &domain.Session{}, "run")

	assert.IsType(t, domain.ThinkingEvent{}, <-sender.events)
	assert.IsType(t, domain.ToolStartEvent{}, <-sender.events)
	assert.IsType(t, domain.ToolEndEvent{}, <-sender.events)
}

func TestRun_MaxIterationsExceeded(t *testing.T) {
	m := &mockLLM{
		id: "infinite",
		streams: []*mockStream{
			{chunks: []mockChunk{{toolCall: &schema.ToolCall{Index: ptr(0), ID: "tc-inf", Function: schema.FunctionCall{Name: "infinite"}}}}},
			{chunks: []mockChunk{{toolCall: &schema.ToolCall{Index: ptr(0), ID: "tc-inf", Function: schema.FunctionCall{Name: "infinite"}}}}},
			{chunks: []mockChunk{{toolCall: &schema.ToolCall{Index: ptr(0), ID: "tc-inf", Function: schema.FunctionCall{Name: "infinite"}}}}},
		},
	}

	mt := &mockTool{
		name: "infinite",
		prepare: func(ctx context.Context, params string) (domain.Invocation, error) {
			return &mockInvocation{content: "kept going"}, nil
		},
	}

	l := NewLoop(m, newMockToolRegistry([]domain.Tool{mt}), 3, nil)

	session := &domain.Session{}
	err := l.Run(context.Background(), session, "go")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max iterations (3) reached")
	lastMsg := session.Messages[len(session.Messages)-1]
	assert.Equal(t, "[Max iterations reached]", lastMsg.Content)
}

func TestRun_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	m := &mockLLM{
		id: "test",
		streams: []*mockStream{
			{chunks: []mockChunk{{text: "ok"}}},
		},
	}

	l := newTestLoop([]domain.Tool{}, m, nil)
	session := &domain.Session{}
	err := l.Run(ctx, session, "hi")

	assert.ErrorIs(t, err, context.Canceled)
	lastMsg := session.Messages[len(session.Messages)-1]
	assert.Equal(t, "[Session cancelled by user]", lastMsg.Content)
}

func TestRun_ParallelToolCalls(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	sender := newMockEventSender(20)

	m := &mockLLM{
		id: "test",
		streams: []*mockStream{
			{chunks: []mockChunk{
				{toolCall: &schema.ToolCall{Index: ptr(0), ID: "tc-1", Function: schema.FunctionCall{Name: "t1"}}},
				{toolCall: &schema.ToolCall{Index: ptr(1), ID: "tc-2", Function: schema.FunctionCall{Name: "t2"}}},
			}},
			{chunks: []mockChunk{{text: "Done."}}},
		},
	}

	t1Started := make(chan struct{})
	t2Started := make(chan struct{})
	canFinish := make(chan struct{})

	mt1 := &mockTool{
		name: "t1",
		prepare: func(ctx context.Context, params string) (domain.Invocation, error) {
			return &mockInvocation{
				execute: func(ctx context.Context) (string, error) {
					close(t1Started)
					<-canFinish
					return "R1", nil
				},
			}, nil
		},
	}
	mt2 := &mockTool{
		name: "t2",
		prepare: func(ctx context.Context, params string) (domain.Invocation, error) {
			return &mockInvocation{
				execute: func(ctx context.Context) (string, error) {
					close(t2Started)
					<-canFinish
					return "R2", nil
				},
			}, nil
		},
	}

	l := newTestLoop([]domain.Tool{mt1, mt2}, m, sender)
	session := &domain.Session{}

	runDone := make(chan error, 1)
	go func() {
		runDone <- l.Run(ctx, session, "run")
	}()

	select {
	case <-t1Started:
		select {
		case <-t2Started:
		case <-time.After(500 * time.Millisecond):
			t.Fatal("t2 did not start in parallel with t1")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("t1 did not start")
	}

	close(canFinish)

	err := <-runDone
	assert.NoError(t, err)

	assert.Equal(t, "tc-1", session.Messages[2].ToolCallID)
	assert.Equal(t, "tc-2", session.Messages[3].ToolCallID)
}

func TestRun_ParallelToolCalls_Cancelled_RecordsAll(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := &mockLLM{
		id: "test",
		streams: []*mockStream{
			{chunks: []mockChunk{
				{toolCall: &schema.ToolCall{Index: ptr(0), ID: "tc-1", Function: schema.FunctionCall{Name: "t1"}}},
				{toolCall: &schema.ToolCall{Index: ptr(1), ID: "tc-2", Function: schema.FunctionCall{Name: "t2"}}},
			}},
		},
	}

	mt1 := &mockTool{
		name: "t1",
		prepare: func(ctx context.Context, params string) (domain.Invocation, error) {
			return &mockInvocation{
				execute: func(ctx context.Context) (string, error) {
					time.Sleep(50 * time.Millisecond)
					cancel()
					return "", ctx.Err()
				},
			}, nil
		},
	}
	mt2 := &mockTool{
		name: "t2",
		prepare: func(ctx context.Context, params string) (domain.Invocation, error) {
			return &mockInvocation{
				execute: func(ctx context.Context) (string, error) {
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

	assert.Equal(t, 5, len(session.Messages))
	m2 := session.Messages[2]
	m3 := session.Messages[3]
	assert.Equal(t, "tc-1", m2.ToolCallID)
	assert.True(t, m2.Extra["tool_error"].(bool))
	assert.Equal(t, "tc-2", m3.ToolCallID)
	assert.True(t, m3.Extra["tool_error"].(bool))
}

func TestRun_ParallelToolCalls_CollidingIndices(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	sender := newMockEventSender(20)

	// Simulate a "buggy" provider like GitHub Gemini bridge
	// Two DIFFERENT tool calls, both claiming Index 0 (or nil index)
	m := &mockLLM{
		id: "buggy-gemini-bridge",
		streams: []*mockStream{
			{chunks: []mockChunk{
				{toolCall: &schema.ToolCall{Index: ptr(0), ID: "tc-1", Function: schema.FunctionCall{Name: "t1"}}},
				{toolCall: &schema.ToolCall{Index: ptr(0), ID: "tc-2", Function: schema.FunctionCall{Name: "t2"}}},
			}},
			{chunks: []mockChunk{{text: "Done."}}},
		},
	}

	mt1 := &mockTool{
		name: "t1",
		prepare: func(ctx context.Context, params string) (domain.Invocation, error) {
			return &mockInvocation{content: "R1"}, nil
		},
	}
	mt2 := &mockTool{
		name: "t2",
		prepare: func(ctx context.Context, params string) (domain.Invocation, error) {
			return &mockInvocation{content: "R2"}, nil
		},
	}

	l := newTestLoop([]domain.Tool{mt1, mt2}, m, sender)
	session := &domain.Session{}

	err := l.Run(ctx, session, "run")
	// RED: Should fail with "ConcatMessages: cannot concat ToolCalls with different tool id"
	// GREEN: Patch in loop.go should stably re-index them to 0 and 1, allowing success.
	assert.NoError(t, err)
}

func TestRun_ParallelToolCalls_SequentialCollidingIndices(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	sender := newMockEventSender(20)

	// Simulate a "Buggy but Sequential" provider (Like GitHub Gemini bridge)
	// Tool 1 starts at Index 0, finishes args.
	// Tool 2 starts at Index 0, finishes args.
	m := &mockLLM{
		id: "sequential-buggy-gemini-bridge",
		streams: []*mockStream{
			{chunks: []mockChunk{
				// Tool 1: Index 0
				{toolCall: &schema.ToolCall{Index: ptr(0), ID: "tc-1", Function: schema.FunctionCall{Name: "t1"}}},
				{toolCall: &schema.ToolCall{Index: ptr(0), Function: schema.FunctionCall{Arguments: `{"a":1}`}}},
				
				// Tool 2: ALSO Index 0 (Collision starts here)
				{toolCall: &schema.ToolCall{Index: ptr(0), ID: "tc-2", Function: schema.FunctionCall{Name: "t2"}}},
				{toolCall: &schema.ToolCall{Index: ptr(0), Function: schema.FunctionCall{Arguments: `{"b":2}`}}},
			}},
			{chunks: []mockChunk{{text: "Done."}}},
		},
	}

	mt1 := &mockTool{
		name: "t1",
		prepare: func(ctx context.Context, params string) (domain.Invocation, error) {
			return &mockInvocation{content: "R1:" + params}, nil
		},
	}
	mt2 := &mockTool{
		name: "t2",
		prepare: func(ctx context.Context, params string) (domain.Invocation, error) {
			return &mockInvocation{content: "R2:" + params}, nil
		},
	}

	l := newTestLoop([]domain.Tool{mt1, mt2}, m, sender)
	session := &domain.Session{}

	err := l.Run(ctx, session, "run")
	assert.NoError(t, err)

	// Verify argument routing
	for _, msg := range session.Messages {
		if msg.Role == schema.Tool {
			if msg.ToolCallID == "tc-1" {
				assert.Contains(t, msg.Content, `{"a":1}`, "tc-1 should have its own args")
				assert.NotContains(t, msg.Content, `{"b":2}`, "tc-1 should NOT steal tc-2's args")
			}
			if msg.ToolCallID == "tc-2" {
				assert.Contains(t, msg.Content, `{"b":2}`, "tc-2 should have its own args")
			}
		}
	}
}
