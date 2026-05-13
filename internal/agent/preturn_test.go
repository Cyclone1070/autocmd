package agent

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

type summarizerSpy struct {
	calls [][]*schema.Message
}

func (s *summarizerSpy) Summarize(ctx context.Context, msgs []*schema.Message) (*schema.Message, error) {
	cp := make([]*schema.Message, len(msgs))
	copy(cp, msgs)
	s.calls = append(s.calls, cp)
	return &schema.Message{Role: schema.User, Content: "spy-summary-body"}, nil
}

type summarizerAlwaysErr struct{}

func (summarizerAlwaysErr) Summarize(ctx context.Context, msgs []*schema.Message) (*schema.Message, error) {
	_ = ctx
	_ = msgs
	return nil, errors.New("summarize failed")
}

type captureEventSender struct {
	updates []domain.UIUpdate
}

func (c *captureEventSender) SendUIUpdate(u domain.UIUpdate) {
	c.updates = append(c.updates, u)
}

func TestGraphPreTurn_Compaction_SummarizeErrorStopsAndPreservesMessages(t *testing.T) {
	events := &captureEventSender{}
	r := &GraphRunner{
		llm:          &mockLLM{contextWindow: 100},
		summarizer:   summarizerAlwaysErr{},
		events:       events,
		maxIteration: 10,
	}
	orig := []*schema.Message{
		{Role: schema.User, Content: "u1"},
		{
			Role:    schema.Assistant,
			Content: "a1",
			ResponseMeta: &schema.ResponseMeta{
				Usage: &schema.TokenUsage{TotalTokens: 200},
			},
		},
		{Role: schema.User, Content: "u2"},
	}
	st := &graphRunState{
		session: &domain.Session{Messages: append([]*schema.Message(nil), orig...)},
	}

	_, err := r.graphPreTurn(context.Background(), st)
	require.Error(t, err)
	require.ErrorContains(t, err, "graph preturn compaction: summarize:")
	require.Len(t, events.updates, 2)
	_, ok := events.updates[0].(domain.SummaryCompactionStartEvent)
	require.True(t, ok)
	end, ok := events.updates[1].(domain.SummaryCompactionEndEvent)
	require.True(t, ok)
	require.Contains(t, end.Error, "summarize failed")
	require.Len(t, st.session.Messages, len(orig))
	require.Equal(t, "u2", st.session.Messages[2].Content)
}

func TestGraphRunner_PreTurn_LogsCompactionTriggered(t *testing.T) {
	var buf bytes.Buffer
	origLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(origLogger)
	slog.Info("test logger marker preturn")

	r := &GraphRunner{
		llm:          &mockLLM{contextWindow: 100},
		summarizer:   &Summarizer{runnable: mockSummaryRunnable{}},
		maxIteration: 10,
	}
	st := &graphRunState{
		session: &domain.Session{
			Messages: []*schema.Message{
				{Role: schema.User, Content: "u1"},
				{
					Role: schema.Assistant,
					Content: "a1",
					ResponseMeta: &schema.ResponseMeta{
						Usage: &schema.TokenUsage{TotalTokens: 200},
					},
				},
				{Role: schema.User, Content: "u2"},
			},
		},
	}

	_, err := r.graphPreTurn(context.Background(), st)
	require.NoError(t, err)
	logOutput := buf.String()
	require.Contains(t, logOutput, "test logger marker preturn", "test logger did not capture output: %q", logOutput)
	require.Contains(t, logOutput, "graph preturn compaction triggered", "expected compaction trigger log, got: %q", logOutput)
	require.Contains(t, logOutput, "graph preturn compaction applied", "expected compaction applied log, got: %q", logOutput)
}

func TestGraphPreTurn_Compaction_UserTailSingleUserMessage(t *testing.T) {
	r := &GraphRunner{
		llm:          &mockLLM{contextWindow: 100},
		summarizer:   &Summarizer{runnable: mockSummaryRunnable{}},
		maxIteration: 10,
	}
	st := &graphRunState{
		session: &domain.Session{
			Messages: []*schema.Message{
				{Role: schema.User, Content: "u1"},
				{
					Role:    schema.Assistant,
					Content: "a1",
					ResponseMeta: &schema.ResponseMeta{
						Usage: &schema.TokenUsage{TotalTokens: 200},
					},
				},
				{Role: schema.User, Content: "current ask"},
			},
		},
	}

	_, err := r.graphPreTurn(context.Background(), st)
	require.NoError(t, err)
	require.Len(t, st.session.Messages, 1, "user tail compaction should collapse to one user message")
	require.Equal(t, schema.User, st.session.Messages[0].Role)
	require.Contains(t, st.session.Messages[0].Content, "[Conversation compacted automatically]")
	require.Contains(t, st.session.Messages[0].Content, "=== CURRENT REQUEST ===")
	require.Contains(t, st.session.Messages[0].Content, "current ask")
	require.Contains(t, st.session.Messages[0].Content, "summary")
}

func TestGraphPreTurn_Compaction_AssistantTailSummarizesFullHistory(t *testing.T) {
	spy := &summarizerSpy{}
	r := &GraphRunner{
		llm:          &mockLLM{contextWindow: 100},
		summarizer:   spy,
		maxIteration: 10,
	}
	st := &graphRunState{
		session: &domain.Session{
			Messages: []*schema.Message{
				{Role: schema.User, Content: "u1"},
				{
					Role:    schema.Assistant,
					Content: "a1",
					ResponseMeta: &schema.ResponseMeta{
						Usage: &schema.TokenUsage{TotalTokens: 200},
					},
				},
			},
		},
	}

	_, err := r.graphPreTurn(context.Background(), st)
	require.NoError(t, err)
	require.Len(t, spy.calls, 1)
	require.Len(t, spy.calls[0], 2, "Summarize must receive full transcript when tail is not user")
	require.Len(t, st.session.Messages, 1)
	require.Equal(t, schema.User, st.session.Messages[0].Role)
	require.Contains(t, st.session.Messages[0].Content, "spy-summary-body")
}

func TestGraphPreTurn_Compaction_ToolTailSummarizesFullHistory(t *testing.T) {
	spy := &summarizerSpy{}
	r := &GraphRunner{
		llm:          &mockLLM{contextWindow: 100},
		summarizer:   spy,
		maxIteration: 10,
	}
	st := &graphRunState{
		session: &domain.Session{
			Messages: []*schema.Message{
				{Role: schema.User, Content: "u1"},
				{
					Role:    schema.Assistant,
					Content: "",
					ToolCalls: []schema.ToolCall{
						{ID: "c1", Function: schema.FunctionCall{Name: "x", Arguments: "{}"}},
					},
					ResponseMeta: &schema.ResponseMeta{
						Usage: &schema.TokenUsage{TotalTokens: 200},
					},
				},
				{Role: schema.Tool, Content: "tool-out", ToolCallID: "c1"},
			},
		},
	}

	_, err := r.graphPreTurn(context.Background(), st)
	require.NoError(t, err)
	require.Len(t, spy.calls, 1)
	require.Len(t, spy.calls[0], 3, "Summarize must receive full transcript when tail is tool")
	require.Len(t, st.session.Messages, 1)
	require.Equal(t, schema.User, st.session.Messages[0].Role)
}

func TestGraphPreTurn_Compaction_EmitsSummaryLifecycleEventsOnSuccess(t *testing.T) {
	events := &captureEventSender{}
	r := &GraphRunner{
		llm:        &mockLLM{contextWindow: 100},
		summarizer: &Summarizer{runnable: mockSummaryRunnable{}},
		events:     events,
		maxIteration: 10,
	}
	st := &graphRunState{
		session: &domain.Session{
			Messages: []*schema.Message{
				{Role: schema.User, Content: "u1"},
				{
					Role:    schema.Assistant,
					Content: "a1",
					ResponseMeta: &schema.ResponseMeta{
						Usage: &schema.TokenUsage{TotalTokens: 200},
					},
				},
				{Role: schema.User, Content: "u2"},
			},
		},
	}

	_, err := r.graphPreTurn(context.Background(), st)
	require.NoError(t, err)
	require.Len(t, events.updates, 2)
	_, ok := events.updates[0].(domain.SummaryCompactionStartEvent)
	require.True(t, ok)
	end, ok := events.updates[1].(domain.SummaryCompactionEndEvent)
	require.True(t, ok)
	require.Empty(t, end.Error)
}
