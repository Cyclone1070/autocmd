package github

import (
	"encoding/json"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestToSDKMessages(t *testing.T) {
	msgs := domain.Messages{
		domain.SystemMessage{Content: "S1"},
		domain.SystemMessage{Content: "S2"},
		domain.UserMessage{Content: "U1"},
		domain.UserMessage{Content: "U2"},
		domain.AssistantMessage{Content: "A1"},
		domain.AssistantMessage{
			Content: "A2",
			ToolCalls: []domain.ToolCall{
				{ID: "call_1", Name: "t1", Arguments: json.RawMessage(`{}`)},
			},
		},
		domain.ToolMessage{ToolCallID: "call_1", Content: "R1"},
	}

	sdkMsgs := toSDKMessages(msgs)

	// Expected normalized:
	// 1. System: S1\n\nS2
	// 2. User: U1\n\nU2
	// 3. Assistant: A1\n\nA2 (with call_1)
	// 4. Tool: call_1
	assert.Len(t, sdkMsgs, 4)

	// Check Assistant Message details
	asst := sdkMsgs[2].OfAssistant
	assert.NotNil(t, asst)
	assert.Equal(t, "A1\n\nA2", asst.Content.OfString.Value)
	assert.Len(t, asst.ToolCalls, 1)
	assert.Equal(t, "call_1", asst.ToolCalls[0].OfFunction.ID)
}

func TestToSDKTools(t *testing.T) {
	decls := []domain.Declaration{
		{
			Name:        "get_weather",
			Description: "Get weather for a city",
			Parameters: &domain.Schema{
				Type: domain.TypeObject,
				Properties: map[string]*domain.Schema{
					"city": {Type: domain.TypeString},
				},
				Required: []string{"city"},
			},
		},
	}

	sdkTools := toSDKTools(decls)
	assert.Len(t, sdkTools, 1)
	tool := sdkTools[0].OfFunction
	assert.Equal(t, "get_weather", tool.Function.Name)
	assert.Equal(t, "Get weather for a city", tool.Function.Description.Value)
	
	// Verify parameters were unmarshaled correctly
	params := tool.Function.Parameters
	assert.Equal(t, "object", params["type"])
}
