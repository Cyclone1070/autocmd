package agent

import (
	"context"
	"testing"
	"time"

	"github.com/Cyclone1070/autocmd/internal/domain"
	"github.com/Cyclone1070/autocmd/internal/runtimectx"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestGraphRunner_Run_ReActToolThenFinalMessage(t *testing.T) {
	ctx := context.Background()
	greet := &greetTool{}
	reg := &testToolRegistry{tools: map[string]tool.BaseTool{testToolNameGreet: greet}}

	info, err := greet.Info(ctx)
	require.NoError(t, err)

	llm := &mockLLM{
		id:            testMockLLMID,
		displayName:   testMockLLMDisplayName,
		contextWindow: 128_000,
		streamErr:     nil,
		streams: []*mockStream{
			{chunks: []mockChunk{{
				toolCalls: []schema.ToolCall{{
					ID: testToolCallIDTC1,
					Function: schema.FunctionCall{
						Name:      info.Name,
						Arguments: `{"name":"Ada"}`,
					},
				}},
			}}},
			{chunks: []mockChunk{{text: "done"}}},
		},
	}

	runner, err := NewGraphRunner(llm, reg, nil, 20, nil, nil, nil, nil)
	require.NoError(t, err)

	sess := &domain.Session{SessionMessages: domain.SessionMessages{Messages: []*schema.Message{}}}
	require.NoError(t, runner.Run(ctx, sess, "say hi"))

	require.GreaterOrEqual(t, greet.invokeCount, 1)
	last := sess.Messages[len(sess.Messages)-1]
	require.Equal(t, schema.Assistant, last.Role)
	require.Contains(t, last.Content, "done")
}

func TestGraphRunner_Run_TextChunkBeforeToolCallStillInvokesTool(t *testing.T) {
	ctx := context.Background()
	greet := &greetTool{}
	reg := &testToolRegistry{tools: map[string]tool.BaseTool{testToolNameGreet: greet}}

	info, err := greet.Info(ctx)
	require.NoError(t, err)

	llm := &mockLLM{
		id:            testMockLLMID,
		displayName:   testMockLLMDisplayName,
		contextWindow: 128_000,
		streamErr:     nil,
		streams: []*mockStream{
			{chunks: []mockChunk{
				{text: "I'll use the tool."},
				{toolCalls: []schema.ToolCall{{
					ID: testToolCallIDTC1,
					Function: schema.FunctionCall{
						Name:      info.Name,
						Arguments: `{"name":"Bob"}`,
					},
				}}},
			}},
			{chunks: []mockChunk{{text: "finished"}}},
		},
	}

	runner, err := NewGraphRunner(llm, reg, nil, 20, nil, nil, nil, nil)
	require.NoError(t, err)

	sess := &domain.Session{SessionMessages: domain.SessionMessages{Messages: []*schema.Message{}}}
	require.NoError(t, runner.Run(ctx, sess, "greet Bob"))

	require.Equal(t, 1, greet.invokeCount, "scan-all checker must route to tools when text precedes tool calls")
}

func TestGraphRunner_Run_EmitsAssistantTextEvents(t *testing.T) {
	ctx := context.Background()
	reg := &testToolRegistry{tools: map[string]tool.BaseTool{testToolNameGreet: &greetTool{}}}
	events := &mockEventSender{}

	llm := &mockLLM{
		id:            testMockLLMID,
		displayName:   testMockLLMDisplayName,
		contextWindow: 128_000,
		streams: []*mockStream{
			{chunks: []mockChunk{{text: "hello from assistant"}}},
		},
	}

	runner, err := NewGraphRunner(llm, reg, nil, 20, events, nil, nil, nil)
	require.NoError(t, err)

	sess := &domain.Session{SessionMessages: domain.SessionMessages{Messages: []*schema.Message{}}}
	require.NoError(t, runner.Run(ctx, sess, "say hi"))

	foundText := false
	for _, upd := range events.updates {
		if te, ok := upd.(domain.TextEvent); ok && te.Text != "" && !te.IsThought {
			foundText = true
			break
		}
	}
	require.True(t, foundText, "expected at least one assistant text event")
}

func TestGraphRunner_Run_EmitsAssistantTextEvents_WhenToolCallTurnContainsText(t *testing.T) {
	ctx := context.Background()
	greet := &greetTool{}
	reg := &testToolRegistry{tools: map[string]tool.BaseTool{testToolNameGreet: greet}}
	events := &mockEventSender{}

	info, err := greet.Info(ctx)
	require.NoError(t, err)

	llm := &mockLLM{
		id:            testMockLLMID,
		displayName:   testMockLLMDisplayName,
		contextWindow: 128_000,
		streams: []*mockStream{
			{chunks: []mockChunk{
				{text: "let me inspect first"},
				{toolCalls: []schema.ToolCall{{
					ID: testToolCallIDTC1,
					Function: schema.FunctionCall{
						Name:      info.Name,
						Arguments: `{"name":"Ada"}`,
					},
				}}},
			}},
			{chunks: []mockChunk{{text: "done"}}},
		},
	}

	runner, err := NewGraphRunner(llm, reg, nil, 20, events, nil, nil, nil)
	require.NoError(t, err)

	sess := &domain.Session{SessionMessages: domain.SessionMessages{Messages: []*schema.Message{}}}
	require.NoError(t, runner.Run(ctx, sess, "say hi"))

	foundToolTurnText := false
	for _, upd := range events.updates {
		te, ok := upd.(domain.TextEvent)
		if ok && te.Text == "let me inspect first" && !te.IsThought {
			foundToolTurnText = true
			break
		}
	}
	require.True(t, foundToolTurnText, "expected assistant text from tool-call turn to be emitted")
}

type statefulMockTaskNotifier struct {
	hasRunning bool
	callCount  int
}

func (m *statefulMockTaskNotifier) Drain() []domain.TaskResult {
	return nil
}

func (m *statefulMockTaskNotifier) HasRunning() bool {
	m.callCount++
	if m.callCount > 1 {
		return false
	}
	return m.hasRunning
}

func TestGraphRunner_Run_TurnGuardPreventsExitWithRunningTasks(t *testing.T) {
	ctx := context.Background()
	greet := &greetTool{}
	reg := &testToolRegistry{tools: map[string]tool.BaseTool{testToolNameGreet: greet}}

	mockNotifier := &statefulMockTaskNotifier{hasRunning: true}
	mockEvents := &mockEventSender{}

	llm := &mockLLM{
		id:            testMockLLMID,
		displayName:   testMockLLMDisplayName,
		contextWindow: 128_000,
		streams: []*mockStream{
			{chunks: []mockChunk{{text: "I want to finish but tasks are running"}}},
			{chunks: []mockChunk{{text: "Ok I'll stop them or wait"}}},
		},
	}

	runner, err := NewGraphRunner(llm, reg, nil, 20, mockEvents, mockNotifier, nil, nil)
	require.NoError(t, err)

	sess := &domain.Session{SessionMessages: domain.SessionMessages{Messages: []*schema.Message{}}}
	require.NoError(t, runner.Run(ctx, sess, "do something"))

	// We expect 4 messages: User, Assistant 1, Synthetic User, Assistant 2
	require.Equal(t, 4, len(sess.Messages), "Expected 4 messages due to turn guard loop")
	require.Equal(t, schema.User, sess.Messages[2].Role)
	require.Contains(t, sess.Messages[2].Content, "Exit denied. Active background tasks exist.")
	require.Contains(t, sess.Messages[2].Content, "Required action: Call")
	require.Contains(t, sess.Messages[2].Content, "<note>Message auto generated")
	require.NotContains(t, sess.Messages[2].Content, "&#xA;")

	// Verify that the system notification event was emitted
	var foundNotification bool
	for _, ev := range mockEvents.updates {
		if se, ok := ev.(domain.SystemNotificationEvent); ok {
			foundNotification = true
			require.Contains(t, se.Content, "Exit denied. Active background tasks exist.")
			break
		}
	}
	require.True(t, foundNotification, "Expected UI to receive SystemNotificationEvent when Turn Guard triggered")
}

type sinkTool struct {
	tool.BaseTool
}

func (t *sinkTool) Name() string { return "sink_tool" }
func (t *sinkTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "sink_tool"}, nil
}
func (t *sinkTool) IsConcurrentSafe() bool { return true }
func (t *sinkTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	if sink, ok := runtimectx.ToolDisplaySinkFrom(ctx); ok {
		time.Sleep(10 * time.Millisecond) // Force overlap!
		sink(argumentsInJSON, domain.NewStringDisplay("sink_tool", "output"))
	}
	return "done", nil
}

func TestGraphRunner_Run_ParallelToolCalls_Race(t *testing.T) {
	ctx := context.Background()
	st := &sinkTool{}
	reg := &testToolRegistry{tools: map[string]tool.BaseTool{"sink_tool": st}}

	llm := &mockLLM{
		id:            testMockLLMID,
		displayName:   testMockLLMDisplayName,
		contextWindow: 128_000,
		streams: []*mockStream{
			{chunks: []mockChunk{{
				toolCalls: []schema.ToolCall{
					{
						ID: "tc1",
						Function: schema.FunctionCall{
							Name:      "sink_tool",
							Arguments: `key1`,
						},
					},
					{
						ID: "tc2",
						Function: schema.FunctionCall{
							Name:      "sink_tool",
							Arguments: `key2`,
						},
					},
				},
			}}},
			{chunks: []mockChunk{{text: "done"}}},
		},
	}

	runner, err := NewGraphRunner(llm, reg, nil, 20, nil, nil, nil, nil)
	require.NoError(t, err)

	sess := &domain.Session{SessionMessages: domain.SessionMessages{Messages: []*schema.Message{}}}

	err = runner.Run(ctx, sess, "use sink_tool")
	require.NoError(t, err)
}
