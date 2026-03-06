package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageJSON_RoundTrip(t *testing.T) {
	messages := []Message{
		UserMessage{Content: "Hello from user"},
		AssistantMessage{
			Content: "Assistant response",
			ToolCalls: []ToolCall{
				{ID: "call-1", Name: "shell", Arguments: json.RawMessage(`{"command":"ls"}`)},
			},
		},
		ToolMessage{
			ToolCallID: "call-1",
			ToolName:   "shell",
			Content:    "file1.txt",
			ToolError:  false,
		},
	}

	// 1. Marshal the slice of interfaces
	data, err := json.Marshal(messages)
	require.NoError(t, err)

	// Verify that "role" field is present in JSON
	var raw []map[string]interface{}
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err)
	assert.Equal(t, "user", raw[0]["role"])
	assert.Equal(t, "assistant", raw[1]["role"])
	assert.Equal(t, "tool", raw[2]["role"])

	// 2. Unmarshal back into interface slice using the helper type
	var decoded Messages
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	require.Len(t, decoded, 3)
	assert.IsType(t, UserMessage{}, decoded[0])
	assert.IsType(t, AssistantMessage{}, decoded[1])
	assert.IsType(t, ToolMessage{}, decoded[2])

	assert.Equal(t, "Hello from user", decoded[0].(UserMessage).Content)
	assert.Equal(t, "Assistant response", decoded[1].(AssistantMessage).Content)
	assert.Equal(t, "call-1", decoded[2].(ToolMessage).ToolCallID)
}
