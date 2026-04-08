package domain

import (
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageJSON_RoundTrip(t *testing.T) {
	messages := []*schema.Message{
		{
			Role:    schema.User,
			Content: "Hello from user",
		},
		{
			Role:    schema.Assistant,
			Content: "Assistant response",
			ToolCalls: []schema.ToolCall{
				{
					ID:   "call-1",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "bash",
						Arguments: `{"command":"ls"}`,
					},
				},
			},
		},
		{
			Role:       schema.Tool,
			ToolCallID: "call-1",
			ToolName:   "bash",
			Content:    "file1.txt",
		},
	}

	displays := ToolDisplays{
		"call-1": BashDisplay{
			TypeField:      "bash",
			Comment:        "Listing files",
			Command:        "ls",
			CapturedOutput: "",
		},
	}

	// 1. Marshal the slice
	data, err := json.Marshal(messages)
	require.NoError(t, err)

	// Verify that "role" field is present in JSON
	var raw []map[string]interface{}
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err)
	assert.Equal(t, "user", raw[0]["role"])
	assert.Equal(t, "assistant", raw[1]["role"])
	assert.Equal(t, "tool", raw[2]["role"])

	// 2. Unmarshal back
	var decodedMessages []*schema.Message
	err = json.Unmarshal(data, &decodedMessages)
	require.NoError(t, err)

	dataDisp, err := json.Marshal(displays)
	require.NoError(t, err)

	var decodedDisplays ToolDisplays
	err = json.Unmarshal(dataDisp, &decodedDisplays)
	require.NoError(t, err)

	require.Len(t, decodedMessages, 3)
	assert.Equal(t, schema.User, decodedMessages[0].Role)
	assert.Equal(t, schema.Assistant, decodedMessages[1].Role)
	assert.Equal(t, schema.Tool, decodedMessages[2].Role)

	assert.Equal(t, "Hello from user", decodedMessages[0].Content)
	assert.Equal(t, "Assistant response", decodedMessages[1].Content)
	assert.Equal(t, "call-1", decodedMessages[2].ToolCallID)

	require.Len(t, decodedDisplays, 1)
	assert.IsType(t, BashDisplay{}, decodedDisplays["call-1"])
	assert.Equal(t, "Listing files", decodedDisplays["call-1"].(BashDisplay).Comment)
}
