package agent

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestGraphRunner_ChatTurn_LogsSpecificLLMRecvError(t *testing.T) {
	var buf bytes.Buffer
	origLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(origLogger)
	slog.Info("test logger marker chat")

	greet := &greetTool{}
	reg := &testToolRegistry{tools: map[string]tool.BaseTool{"greet": greet}}
	llm := &mockLLM{
		id:            "mock",
		displayName:   "Mock",
		contextWindow: 128_000,
		streams: []*mockStream{
			{chunks: []mockChunk{{text: "partial"}}, err: errors.New("Error 500, Message: Internal error encountered.")},
		},
	}
	runner, err := NewGraphRunner(llm, reg, nil, 20, nil, nil, nil, nil)
	require.NoError(t, err)

	st := &graphRunState{
		session: &domain.Session{
			Messages: []*schema.Message{
				{Role: schema.User, Content: "hi"},
			},
		},
	}
	_, err = runner.graphChatTurn(context.Background(), st)
	require.Error(t, err)
	logOutput := buf.String()
	require.Contains(t, logOutput, "test logger marker chat", "test logger did not capture output: %q", logOutput)
	require.Contains(t, logOutput, "graph chat stream recv failed")
	require.True(t, strings.Contains(logOutput, "Error 500, Message: Internal error encountered.") || strings.Contains(logOutput, "Internal error encountered"))
}

func TestNormalizeToolCallIndices_AssignsStableUniqueIndicesByID(t *testing.T) {
	idx0 := 0
	idx1 := 1
	chunks := []*schema.Message{
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{
				{ID: "a", Index: &idx0, Function: schema.FunctionCall{Name: "read_file"}},
				{ID: "b", Index: &idx0, Function: schema.FunctionCall{Name: "grep"}},
			},
		},
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{
				{ID: "a", Index: &idx1, Function: schema.FunctionCall{Name: "read_file"}},
				{ID: "b", Index: &idx1, Function: schema.FunctionCall{Name: "grep"}},
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
				{ID: "a", Index: &idx5, Function: schema.FunctionCall{Name: "read_file"}},
			},
		},
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{
				{Index: &idx5, Function: schema.FunctionCall{Name: "read_file"}},
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
				{Index: &orig, Function: schema.FunctionCall{Name: "grep"}},
			},
		},
	}

	normalizeToolCallIndices(chunks)

	require.NotNil(t, chunks[0].ToolCalls[0].Index)
	require.Equal(t, 42, *chunks[0].ToolCalls[0].Index)
}

func TestGraphRunner_ChatTurn_EmitsOnlyThoughtTextEvents(t *testing.T) {
	ctx := context.Background()
	reg := &testToolRegistry{tools: map[string]tool.BaseTool{"greet": &greetTool{}}}
	events := &mockEventSender{}

	llm := &mockLLM{
		id:            "mock",
		displayName:   "Mock",
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
			Messages: []*schema.Message{
				{Role: schema.User, Content: "think first"},
			},
		},
	}
	_, err = runner.graphChatTurn(ctx, st)
	require.NoError(t, err)

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
