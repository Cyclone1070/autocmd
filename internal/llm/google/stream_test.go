package google

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
	"google.golang.org/genai"
)

func TestToChunks_Thought(t *testing.T) {
	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{
							Thought:          true,
							ThoughtSignature: []byte("sig-123"),
							Text:             "I am thinking",
						},
						{
							Text: "Hello",
						},
					},
				},
			},
		},
	}

	chunks := toChunks(resp)
	assert.Len(t, chunks, 2)

	// Since my StreamChunk doesn't have Thought yet, I'll expect it in domain.TextChunk if we merge it?
	// Actually, the plan says updating AssistantMessage and ToolCall.
	// For streaming, a TextChunk could carry Thought flag?
	// Or maybe a new domain.ThoughtChunk?
	
	// Let's assume TextChunk doesn't have it yet.
	// Wait, if I'm in stream.go, I should append it to the ToolCall if it's there.
	// But in this test case, there is NO ToolCall, just Thought.
	
	// If it's a Thought part, we should probably record it.
}

func TestToChunks_ToolCall_WithSignature(t *testing.T) {
	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{
							ThoughtSignature: []byte("sig-abc"),
							FunctionCall: &genai.FunctionCall{
								Name: "my_tool",
								Args: map[string]any{"x": 1},
							},
						},
					},
				},
			},
		},
	}

	chunks := toChunks(resp)
	assert.Len(t, chunks, 1)
	tc, ok := chunks[0].(domain.ToolCall)
	assert.True(t, ok)
	assert.Equal(t, "my_tool", tc.Name)
	// We expect the ToolCall struct to have ThoughtSignature
	assert.Equal(t, "sig-abc", tc.ThoughtSignature)
}
