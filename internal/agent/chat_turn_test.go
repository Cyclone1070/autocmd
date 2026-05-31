package agent

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/Cyclone1070/autocmd/internal/domain"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestGraphRunner_ChatTurn_IncludesSystemPrompt(t *testing.T) {
	reg := &testToolRegistry{tools: map[string]tool.BaseTool{testToolNameGreet: &greetTool{}}}
	llm := &mockLLM{
		id:            testMockLLMID,
		displayName:   testMockLLMDisplayName,
		contextWindow: 128_000,
		streams: []*mockStream{
			{chunks: []mockChunk{{text: "ok"}}},
		},
	}
	runner, err := NewGraphRunner(llm, reg, nil, 20, nil, nil, nil, nil)
	require.NoError(t, err)

	sessionMsgs := []*schema.Message{
		{Role: schema.User, Content: "hello"},
	}
	beforeLen := len(sessionMsgs)

	st := &graphRunState{
		session: &domain.Session{
			SessionMessages: domain.SessionMessages{
				Messages: sessionMsgs,
			},
		},
	}

	_, err = runner.graphChatTurn(context.Background(), st)
	require.NoError(t, err)

	require.NotNil(t, llm.LastMessages, "LLM should have received messages")
	require.GreaterOrEqual(t, len(llm.LastMessages), 1, "LLM should have received at least one message")
	require.Equal(t, schema.System, llm.LastMessages[0].Role, "first message to LLM should be system prompt")
	require.Equal(t, systemPrompt, llm.LastMessages[0].Content, "system prompt content should match")

	// Session should contain the original user message
	require.Greater(t, len(st.session.Messages), 0, "session should have messages")
	require.Equal(t, schema.User, st.session.Messages[0].Role, "session starts with user message")
	require.Equal(t, "hello", st.session.Messages[0].Content, "session user message preserved")
	for _, m := range st.session.Messages {
		require.NotEqual(t, schema.System, m.Role, "system messages should not leak into session")
	}
	// Session gained the assistant response
	require.GreaterOrEqual(t, len(st.session.Messages), beforeLen, "session should have at least original messages")
}

func TestGraphRunner_ChatTurn_LogsSpecificLLMRecvError(t *testing.T) {
	var buf bytes.Buffer
	origLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(origLogger)
	slog.Info("test logger marker chat")

	greet := &greetTool{}
	reg := &testToolRegistry{tools: map[string]tool.BaseTool{testToolNameGreet: greet}}
	llm := &mockLLM{
		id:            testMockLLMID,
		displayName:   testMockLLMDisplayName,
		contextWindow: 128_000,
		streams: []*mockStream{
			{chunks: []mockChunk{{text: "partial"}}, err: errors.New("internal error encountered")},
		},
	}
	runner, err := NewGraphRunner(llm, reg, nil, 20, nil, nil, nil, nil)
	require.NoError(t, err)

	st := &graphRunState{
		session: &domain.Session{
			SessionMessages: domain.SessionMessages{
				Messages: []*schema.Message{
					{Role: schema.User, Content: "hi"},
				},
			},
		},
	}
	_, err = runner.graphChatTurn(context.Background(), st)
	require.Error(t, err)
	logOutput := buf.String()
	require.Contains(t, logOutput, "test logger marker chat", "test logger did not capture output: %q", logOutput)
	require.Contains(t, logOutput, "graph chat stream recv failed")
	require.Contains(t, logOutput, "internal error encountered")
}

func TestNormalizeToolCallIndices_AssignsStableUniqueIndicesByID(t *testing.T) {
	idx0 := 0
	idx1 := 1
	chunks := []*schema.Message{
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{
				{ID: "a", Index: &idx0, Function: schema.FunctionCall{Name: testToolNameReadFile}},
				{ID: "b", Index: &idx0, Function: schema.FunctionCall{Name: testToolNameGrep}},
			},
		},
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{
				{ID: "a", Index: &idx1, Function: schema.FunctionCall{Name: testToolNameReadFile}},
				{ID: "b", Index: &idx1, Function: schema.FunctionCall{Name: testToolNameGrep}},
			},
		},
	}

	normalizeToolCallIndices(chunks)

	require.NotNil(t, chunks[0].ToolCalls[0].Index)
	require.NotNil(t, chunks[0].ToolCalls[1].Index)
	require.Equal(t, *chunks[0].ToolCalls[0].Index, *chunks[1].ToolCalls[0].Index, "same ID must keep same fixed index")
	require.Equal(t, *chunks[0].ToolCalls[1].Index, *chunks[1].ToolCalls[1].Index, "same ID must keep same fixed index")
	require.NotEqual(t, *chunks[0].ToolCalls[0].Index, *chunks[0].ToolCalls[1].Index, "different IDs must have unique indices")
}

func TestNormalizeToolCallIndices_MapsIndexOnlyChunksAfterIDChunk(t *testing.T) {
	idx5 := 5
	chunks := []*schema.Message{
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{
				{ID: "a", Index: &idx5, Function: schema.FunctionCall{Name: testToolNameReadFile}},
			},
		},
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{
				{Index: &idx5, Function: schema.FunctionCall{Name: testToolNameReadFile}},
			},
		},
	}

	normalizeToolCallIndices(chunks)

	require.NotNil(t, chunks[0].ToolCalls[0].Index)
	require.NotNil(t, chunks[1].ToolCalls[0].Index)
	require.Equal(t, *chunks[0].ToolCalls[0].Index, *chunks[1].ToolCalls[0].Index)
}

func TestNormalizeToolCallIndices_LeavesUnknownIndexOnlyChunkUnchanged(t *testing.T) {
	orig := 42
	chunks := []*schema.Message{
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{
				{Index: &orig, Function: schema.FunctionCall{Name: testToolNameGrep}},
			},
		},
	}

	normalizeToolCallIndices(chunks)

	require.NotNil(t, chunks[0].ToolCalls[0].Index)
	require.Equal(t, 42, *chunks[0].ToolCalls[0].Index)
}

func TestGraphRunner_ChatTurn_EmitsOnlyThoughtTextEvents(t *testing.T) {
	ctx := context.Background()
	reg := &testToolRegistry{tools: map[string]tool.BaseTool{testToolNameGreet: &greetTool{}}}
	events := &mockEventSender{}

	llm := &mockLLM{
		id:            testMockLLMID,
		displayName:   testMockLLMDisplayName,
		contextWindow: 128_000,
		streams: []*mockStream{
			{chunks: []mockChunk{
				{reasoningContent: "first thought"},
				{reasoningContent: " second thought"},
				{text: "final answer"},
			}},
		},
	}

	runner, err := NewGraphRunner(llm, reg, nil, 20, events, nil, nil, nil)
	require.NoError(t, err)

	st := &graphRunState{
		session: &domain.Session{
			SessionMessages: domain.SessionMessages{
				Messages: []*schema.Message{
					{Role: schema.User, Content: "think first"},
				},
			},
		},
	}
	_, err = runner.graphChatTurn(ctx, st)
	require.NoError(t, err)

	last := lastAssistant(st.session.Messages)
	require.NotNil(t, last)
	_, has := last.Extra[domain.ThoughtDurationMsExtraKey]
	require.True(t, has, "thought duration should be persisted in Message.Extra for history")

	thoughtTextCount := 0
	nonTextEventCount := 0
	for _, upd := range events.updates {
		if e, ok := upd.(domain.TextEvent); ok {
			if e.IsThought {
				thoughtTextCount++
			}
			continue
		}
		nonTextEventCount++
	}
	require.Equal(t, 0, nonTextEventCount, "unified contract should emit only text deltas for this scenario")
	require.Equal(t, 2, thoughtTextCount, "thought chunks should still emit thought text deltas")
}

func TestBuildConcatenatedAssistantMessage_EmptyChunks(t *testing.T) {
	msg, err := buildConcatenatedAssistantMessage(nil)
	require.NoError(t, err)
	require.Nil(t, msg)

	msg, err = buildConcatenatedAssistantMessage([]*schema.Message{})
	require.NoError(t, err)
	require.Nil(t, msg)
}

func TestBuildConcatenatedAssistantMessage_StripsIncompleteToolCalls(t *testing.T) {
	idx0 := 0
	chunks := []*schema.Message{
		{Role: schema.Assistant, Content: "Let me check that file:"},
		{Role: schema.Assistant, ToolCalls: []schema.ToolCall{
			{ID: "call_incomplete", Type: "function", Index: &idx0, Function: schema.FunctionCall{
				Name:      testToolNameReadFile,
				Arguments: `{"description": "Remove in`,
			}},
		}},
	}
	msg, err := buildConcatenatedAssistantMessage(chunks)
	require.NoError(t, err)
	require.NotNil(t, msg)
	require.Equal(t, "Let me check that file:", msg.Content)
	require.Empty(t, msg.ToolCalls, "incomplete tool call should be stripped")
}

func TestBuildConcatenatedAssistantMessage_PreservesCompleteToolCalls(t *testing.T) {
	idx0 := 0
	chunks := []*schema.Message{
		{Role: schema.Assistant, Content: "Let me check that file:"},
		{Role: schema.Assistant, ToolCalls: []schema.ToolCall{
			{ID: "call_valid", Type: "function", Index: &idx0, Function: schema.FunctionCall{
				Name:      testToolNameReadFile,
				Arguments: testReadFileArgsJSON,
			}},
		}},
	}
	msg, err := buildConcatenatedAssistantMessage(chunks)
	require.NoError(t, err)
	require.NotNil(t, msg)
	require.Equal(t, "Let me check that file:", msg.Content)
	require.Len(t, msg.ToolCalls, 1)
	require.Equal(t, "call_valid", msg.ToolCalls[0].ID)
}

func TestBuildConcatenatedAssistantMessage_PreservesTextOnMixedChunks(t *testing.T) {
	idx0 := 0
	idx1 := 1
	chunks := []*schema.Message{
		{Role: schema.Assistant, Content: "Let me check that file:"},
		{Role: schema.Assistant, ToolCalls: []schema.ToolCall{
			{ID: "call_valid", Type: "function", Index: &idx0, Function: schema.FunctionCall{
				Name:      testToolNameReadFile,
				Arguments: testReadFileArgsJSON,
			}},
			{ID: "call_incomplete", Type: "function", Index: &idx1, Function: schema.FunctionCall{
				Name:      testToolNameGrep,
				Arguments: `{"pattern": "foo`,
			}},
		}},
	}
	msg, err := buildConcatenatedAssistantMessage(chunks)
	require.NoError(t, err)
	require.NotNil(t, msg)
	require.Equal(t, "Let me check that file:", msg.Content)
	require.Len(t, msg.ToolCalls, 1)
	require.Equal(t, "call_valid", msg.ToolCalls[0].ID)
}

func TestApplyThoughtDurationExtra_SetsExtraOnConcatenatedMessage(t *testing.T) {
	chunks := []*schema.Message{
		{Role: schema.Assistant, ReasoningContent: "a"},
		{Role: schema.Assistant, Content: "body"},
	}
	msg, err := buildConcatenatedAssistantMessage(chunks)
	require.NoError(t, err)
	require.NotNil(t, msg)
	require.NotEmpty(t, msg.ReasoningContent)

	start := time.Now().Add(-2 * time.Second)
	end := time.Now()
	applyThoughtDurationExtra(msg, true, start, end)

	v, ok := msg.Extra[domain.ThoughtDurationMsExtraKey]
	require.True(t, ok)
	ms, ok := v.(int64)
	require.True(t, ok, "expected int64 milliseconds, got %T", v)
	require.GreaterOrEqual(t, ms, int64(1500), "duration should reflect wall clock between start and end")
}
