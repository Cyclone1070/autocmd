package domain

import (
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
)

func TestSession_TotalTokens(t *testing.T) {
	tests := []struct {
		name     string
		messages []*schema.Message
		expected int
	}{
		{
			name:     "Empty session",
			messages: []*schema.Message{},
			expected: 0,
		},
		{
			name: "Last assistant message has usage",
			messages: []*schema.Message{
				{Role: schema.User, Content: "Hello"},
				{
					Role:    schema.Assistant,
					Content: "Hi there!",
					ResponseMeta: &schema.ResponseMeta{
						Usage: &schema.TokenUsage{
							TotalTokens: 15,
						},
					},
				},
			},
			expected: 15,
		},
		{
			name: "New user message after assistant response",
			messages: []*schema.Message{
				{Role: schema.User, Content: "Hello"},
				{
					Role:    schema.Assistant,
					Content: "Hi there!",
					ResponseMeta: &schema.ResponseMeta{
						Usage: &schema.TokenUsage{
							TotalTokens: 15,
						},
					},
				},
				{Role: schema.User, Content: "How are you?"}, // Should be ignored
			},
			expected: 15,
		},
		{
			name: "Multiple messages with latest as assistant",
			messages: []*schema.Message{
				{Role: schema.User, Content: "Hello"},
				{
					Role:    schema.Assistant,
					Content: "Response 1",
					ResponseMeta: &schema.ResponseMeta{
						Usage: &schema.TokenUsage{TotalTokens: 20},
					},
				},
				{Role: schema.User, Content: "Follow up"},
				{
					Role:    schema.Assistant,
					Content: "Response 2",
					ResponseMeta: &schema.ResponseMeta{
						Usage: &schema.TokenUsage{TotalTokens: 45},
					},
				},
			},
			expected: 45,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Session{SessionMessages: SessionMessages{Messages: tt.messages}}
			assert.Equal(t, tt.expected, s.TotalTokens())
		})
	}
}
