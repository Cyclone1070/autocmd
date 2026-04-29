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

// --- Mocks ---

type mockLLM struct {
	streamErr     error
	id            string
	displayName   string
	streams       []*mockStream
	contextWindow int
}

func (m *mockLLM) ID() string          { return m.id }
func (m *mockLLM) DisplayName() string { return m.displayName }
func (m *mockLLM) ContextWindow() int  { return m.contextWindow }

func (m *mockLLM) ComputeTokens(_ context.Context, _ []*schema.Message) (int, error) {
	return 100, nil
}

func (m *mockLLM) Model() model.ToolCallingChatModel {
	return &mockEinoModelBridge{llm: m}
}

// mockEinoModelBridge adapts the old mockLLM.Stream for the new loop.go.
type mockEinoModelBridge struct {
	llm *mockLLM
}

func (b *mockEinoModelBridge) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return nil, nil
}

func (b *mockEinoModelBridge) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
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

func (b *mockEinoModelBridge) WithTools(_ []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return b, nil
}

type mockChunk struct {
	toolCall  *schema.ToolCall
	text      string
	isThought bool
}

type mockStream struct {
	err    error
	chunks []mockChunk
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

// Ensure mockEventSender implements local eventSender.
var _ eventSender = (*mockEventSender)(nil)

func newMockEventSender(size int) *mockEventSender {
	return &mockEventSender{events: make(chan domain.UIUpdate, size)}
}

func newTestLoop(tools []domain.Tool, m domain.LLM, events eventSender) *Loop {
	registry := newMockToolRegistry(tools)
	executor := NewToolExecutor(registry, nil)
	return NewLoop(m, executor, 5, events, &mockTaskNotifier{})
}

type mockTaskNotifier struct {
	notifications []string
}

func (m *mockTaskNotifier) Drain() []string {
	n := m.notifications
	m.notifications = nil
	return n
}

func TestRun_TaskNotificationInjection(t *testing.T) {
	ctx := context.Background()
	m := &mockLLM{
		id: "test",
		streams: []*mockStream{
			{chunks: []mockChunk{{text: "I see the task finished."}}},
		},
	}

	notifier := &mockTaskNotifier{
		notifications: []string{"<task-notification>done</task-notification>"},
	}

	session := &domain.Session{
		Messages: []*schema.Message{
			{Role: schema.User, Content: "Wait for it"},
		},
	}

	registry := newMockToolRegistry(nil)
	executor := NewToolExecutor(registry, nil)
	// This will fail to compile as NewLoop only takes 4 args
	l := NewLoop(m, executor, 5, nil, notifier)

	err := l.Run(ctx, session, "Next")
	assert.NoError(t, err)

	// Check messages:
	// 0: [User] Wait for it
	// 1: [User] Next (Appended by Run)
	// 2: [User] <task-notification> (Injected from Drain)
	// 3: [Assistant] I see...
	assert.Equal(t, 4, len(session.Messages))
	notifMsg := session.Messages[2]
	assert.Equal(t, schema.User, notifMsg.Role)
	assert.Equal(t, "<task-notification>done</task-notification>", notifMsg.Content)
	assert.Equal(t, true, notifMsg.Extra[domain.NotificationMessageExtraKey])
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

	assert.Equal(t, domain.TextEvent{Text: "Hello!"}, <-sender.events)
}

func TestRun_SingleToolCall(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	m := &mockLLM{
		id: "test",
		streams: []*mockStream{
			{chunks: []mockChunk{
				{toolCall: &schema.ToolCall{Index: new(0), ID: "tc-1", Function: schema.FunctionCall{Name: "get_weather"}}},
			}},
			{chunks: []mockChunk{
				{text: "It's sunny!"},
			}},
		},
	}

	mt := &mockTool{
		name: "get_weather",
		prepare: func(_ string) (domain.Invocation, error) {
			return &mockInvocation{content: "Sunny", display: domain.NewStringDisplay("", "weather")}, nil
		},
	}

	session := &domain.Session{}
	sender := newMockEventSender(10)
	l := newTestLoop([]domain.Tool{mt}, m, sender)
	err := l.Run(ctx, session, "Weather?")

	assert.NoError(t, err)
	assert.Equal(t, 4, len(session.Messages))

	assert.IsType(t, domain.ToolStartEvent{}, <-sender.events)
	assert.IsType(t, domain.ToolEndEvent{}, <-sender.events)
	assert.Equal(t, domain.TextEvent{Text: "It's sunny!"}, <-sender.events)
}

func TestRun_ToolStreaming_Events(t *testing.T) {
	ctx := context.Background()
	m := &mockLLM{
		id: "test",
		streams: []*mockStream{
			{chunks: []mockChunk{
				{toolCall: &schema.ToolCall{Index: new(0), ID: "tc-stream", Function: schema.FunctionCall{Name: "bash"}}},
			}},
		},
	}

	mt := &mockTool{
		name: "bash",
		prepare: func(_ string) (domain.Invocation, error) {
			return &mockInvocation{
				display: domain.NewBashDisplay("Run Bash", "echo chunk", "", ""),
				execute: func(_ context.Context) (string, domain.ToolDisplay) {
					return "done", domain.NewBashDisplay("Run Bash", "echo chunk", "", "")
				},
			}, nil
		},
	}

	sender := newMockEventSender(10)
	l := newTestLoop([]domain.Tool{mt}, m, sender)
	_ = l.Run(ctx, &domain.Session{}, "run")

	assert.IsType(t, domain.ToolStartEvent{}, <-sender.events)
	assert.IsType(t, domain.ToolEndEvent{}, <-sender.events)
}

func TestRun_ThinkingEvent_OnlyOnReasoningContent(t *testing.T) {
	ctx := context.Background()
	m := &mockLLM{
		id: "test",
		streams: []*mockStream{
			{chunks: []mockChunk{
				{text: "internal", isThought: true},
				{text: "final"},
			}},
		},
	}

	session := &domain.Session{}
	sender := newMockEventSender(10)
	l := newTestLoop([]domain.Tool{}, m, sender)
	err := l.Run(ctx, session, "Hi")

	assert.NoError(t, err)
	assert.IsType(t, domain.ThinkingEvent{}, <-sender.events)
	assert.Equal(t, domain.TextEvent{Text: "internal", IsThought: true}, <-sender.events)
	assert.Equal(t, domain.TextEvent{Text: "final"}, <-sender.events)
}

func TestRun_MaxIterationsExceeded(t *testing.T) {
	m := &mockLLM{
		id: "infinite",
		streams: []*mockStream{
			{chunks: []mockChunk{{toolCall: &schema.ToolCall{Index: new(0), ID: "tc-inf", Function: schema.FunctionCall{Name: "infinite"}}}}},
			{chunks: []mockChunk{{toolCall: &schema.ToolCall{Index: new(0), ID: "tc-inf", Function: schema.FunctionCall{Name: "infinite"}}}}},
			{chunks: []mockChunk{{toolCall: &schema.ToolCall{Index: new(0), ID: "tc-inf", Function: schema.FunctionCall{Name: "infinite"}}}}},
		},
	}

	mt := &mockTool{
		name: "infinite",
		prepare: func(_ string) (domain.Invocation, error) {
			return &mockInvocation{content: "kept going", display: domain.NewStringDisplay("", "")}, nil
		},
	}

	l := NewLoop(m, NewToolExecutor(newMockToolRegistry([]domain.Tool{mt}), nil), 3, nil, &mockTaskNotifier{})

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
	assert.Equal(t, true, lastMsg.Extra[domain.CancelMessageExtraKey])
}

func TestRun_ContextCancelled_PersistsPartialAssistantProgress(t *testing.T) {
	ctx := context.Background()
	m := &mockLLM{
		id: "test",
		streams: []*mockStream{
			{
				chunks: []mockChunk{
					{text: "partial "},
					{text: "assistant output"},
				},
				err: context.Canceled,
			},
		},
	}

	l := newTestLoop([]domain.Tool{}, m, nil)
	session := &domain.Session{}
	err := l.Run(ctx, session, "hi")

	assert.Error(t, err)
	assert.Len(t, session.Messages, 2)
	assert.Equal(t, schema.User, session.Messages[0].Role)
	assert.Equal(t, schema.Assistant, session.Messages[1].Role)
	assert.Equal(t, "partial assistant output", session.Messages[1].Content)
}

func TestRun_ParallelToolCalls(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	sender := newMockEventSender(20)

	m := &mockLLM{
		id: "test",
		streams: []*mockStream{
			{chunks: []mockChunk{
				{toolCall: &schema.ToolCall{Index: new(0), ID: "tc-1", Function: schema.FunctionCall{Name: "t1"}}},
				{toolCall: &schema.ToolCall{Index: new(1), ID: "tc-2", Function: schema.FunctionCall{Name: "t2"}}},
			}},
			{chunks: []mockChunk{{text: "Done."}}},
		},
	}

	t1Started := make(chan struct{})
	t2Started := make(chan struct{})
	canFinish := make(chan struct{})

	mt1 := &mockTool{
		name: "t1",
		prepare: func(_ string) (domain.Invocation, error) {
			return &mockInvocation{
				display: domain.NewStringDisplay("", ""),
				execute: func(_ context.Context) (string, domain.ToolDisplay) {
					close(t1Started)
					<-canFinish
					return "R1", domain.NewStringDisplay("", "")
				},
			}, nil
		},
	}
	mt2 := &mockTool{
		name: "t2",
		prepare: func(_ string) (domain.Invocation, error) {
			return &mockInvocation{
				display: domain.NewStringDisplay("", ""),
				execute: func(_ context.Context) (string, domain.ToolDisplay) {
					close(t2Started)
					<-canFinish
					return "R2", domain.NewStringDisplay("", "")
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
				{toolCall: &schema.ToolCall{Index: new(0), ID: "tc-1", Function: schema.FunctionCall{Name: "t1"}}},
				{toolCall: &schema.ToolCall{Index: new(1), ID: "tc-2", Function: schema.FunctionCall{Name: "t2"}}},
			}},
		},
	}

	mt1 := &mockTool{
		name: "t1",
		prepare: func(_ string) (domain.Invocation, error) {
			cancelDisp := domain.NewStringDisplay("", "")
			cancelDisp.Error = domain.ToolErrorCancelled
			return &mockInvocation{
				display: cancelDisp,
				execute: func(_ context.Context) (string, domain.ToolDisplay) {
					time.Sleep(50 * time.Millisecond)
					cancel()
					return domain.ToolErrorCancelled, cancelDisp
				},
			}, nil
		},
	}
	mt2 := &mockTool{
		name: "t2",
		prepare: func(_ string) (domain.Invocation, error) {
			cancelDisp := domain.NewStringDisplay("", "")
			cancelDisp.Error = domain.ToolErrorCancelled
			return &mockInvocation{
				display: cancelDisp,
				execute: func(ctx context.Context) (string, domain.ToolDisplay) {
					<-ctx.Done()
					return domain.ToolErrorCancelled, cancelDisp
				},
			}, nil
		},
	}

	l := newTestLoop([]domain.Tool{mt1, mt2}, m, nil)
	session := &domain.Session{}

	err := l.Run(ctx, session, "run")
	assert.ErrorIs(t, err, context.Canceled)

	// Even on fatal cancellation, loop persists tool-result messages so each ToolCallID
	// still has a matching schema.Tool response message.
	assert.Equal(t, 5, len(session.Messages))
	assert.Equal(t, schema.Tool, session.Messages[2].Role)
	assert.Equal(t, "tc-1", session.Messages[2].ToolCallID)
	assert.Equal(t, domain.ToolErrorCancelled, session.Messages[2].Content)
	assert.Equal(t, schema.Tool, session.Messages[3].Role)
	assert.Equal(t, "tc-2", session.Messages[3].ToolCallID)
	assert.Equal(t, domain.ToolErrorCancelled, session.Messages[3].Content)
	assert.Equal(t, "[Session cancelled by user]", session.Messages[4].Content)
	assert.Equal(t, true, session.Messages[4].Extra[domain.CancelMessageExtraKey])
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
				{toolCall: &schema.ToolCall{Index: new(0), ID: "tc-1", Function: schema.FunctionCall{Name: "t1"}}},
				{toolCall: &schema.ToolCall{Index: new(0), ID: "tc-2", Function: schema.FunctionCall{Name: "t2"}}},
			}},
			{chunks: []mockChunk{{text: "Done."}}},
		},
	}

	mt1 := &mockTool{
		name: "t1",
		prepare: func(_ string) (domain.Invocation, error) {
			return &mockInvocation{content: "R1", display: domain.NewStringDisplay("", "")}, nil
		},
	}
	mt2 := &mockTool{
		name: "t2",
		prepare: func(_ string) (domain.Invocation, error) {
			return &mockInvocation{content: "R2", display: domain.NewStringDisplay("", "")}, nil
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
				{toolCall: &schema.ToolCall{Index: new(0), ID: "tc-1", Function: schema.FunctionCall{Name: "t1"}}},
				{toolCall: &schema.ToolCall{Index: new(0), Function: schema.FunctionCall{Arguments: `{"a":1}`}}},

				// Tool 2: ALSO Index 0 (Collision starts here)
				{toolCall: &schema.ToolCall{Index: new(0), ID: "tc-2", Function: schema.FunctionCall{Name: "t2"}}},
				{toolCall: &schema.ToolCall{Index: new(0), Function: schema.FunctionCall{Arguments: `{"b":2}`}}},
			}},
			{chunks: []mockChunk{{text: "Done."}}},
		},
	}

	mt1 := &mockTool{
		name: "t1",
		prepare: func(params string) (domain.Invocation, error) {
			return &mockInvocation{content: "R1:" + params, display: domain.NewStringDisplay("", "")}, nil
		},
	}
	mt2 := &mockTool{
		name: "t2",
		prepare: func(params string) (domain.Invocation, error) {
			return &mockInvocation{content: "R2:" + params, display: domain.NewStringDisplay("", "")}, nil
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

func TestRun_ContextCancelled_PersistsPartialWithCollidingToolIndices(t *testing.T) {
	ctx := context.Background()
	m := &mockLLM{
		id: "buggy-cancelled-stream",
		streams: []*mockStream{
			{
				chunks: []mockChunk{
					{toolCall: &schema.ToolCall{Index: new(0), ID: "tc-1", Function: schema.FunctionCall{Name: "t1"}}},
					{toolCall: &schema.ToolCall{Index: new(0), ID: "tc-2", Function: schema.FunctionCall{Name: "t2"}}},
				},
				err: context.Canceled,
			},
		},
	}

	l := newTestLoop([]domain.Tool{}, m, nil)
	session := &domain.Session{}
	err := l.Run(ctx, session, "run")

	assert.Error(t, err)
	assert.Len(t, session.Messages, 2)
	assert.Equal(t, schema.Assistant, session.Messages[1].Role)
	assert.Len(t, session.Messages[1].ToolCalls, 2)
}
