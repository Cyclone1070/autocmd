package agent

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

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
