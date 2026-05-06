package agent

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
)

func TestSummarizer_Summarize(t *testing.T) {
	ctx := context.Background()
	m := &mockLLM{
		id: "test",
		streams: []*mockStream{
			{chunks: []mockChunk{{text: "<analysis>Thinking...</analysis>\n<summary>\nSummary: The user wants to add context summary.\n</summary>"}}},
		},
	}

	// This is the expected interface/constructor for the summarizer
	s := NewSummarizer(m)

	msgs := []*schema.Message{
		{Role: schema.User, Content: "How do I add context summary?"},
		{Role: schema.Assistant, Content: "You can use a summarizer component."},
	}

	summary, err := s.Summarize(ctx, msgs)
	assert.NoError(t, err)
	assert.NotNil(t, summary)
	assert.Equal(t, schema.User, summary.Role)
	assert.NotContains(t, summary.Content, "<analysis>")
	assert.NotContains(t, summary.Content, "<summary>")
	assert.Contains(t, summary.Content, "Summary: The user wants to add context summary.")
}

func TestSummarizer_Summarize_Empty(t *testing.T) {
	ctx := context.Background()
	s := NewSummarizer(&mockLLM{})

	summary, err := s.Summarize(ctx, nil)
	assert.Error(t, err)
	assert.Nil(t, summary)
}
