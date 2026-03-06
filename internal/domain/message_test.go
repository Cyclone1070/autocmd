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

	displays := ToolDisplays{
		"call-1": ShellDisplay{
			TypeField:      "shell",
			Comment:        "Listing files",
			Command:        "ls",
			CapturedOutput: nil,
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
	var decodedMessages Messages
	err = json.Unmarshal(data, &decodedMessages)
	require.NoError(t, err)

	dataDisp, err := json.Marshal(displays)
	require.NoError(t, err)

	var decodedDisplays ToolDisplays
	err = json.Unmarshal(dataDisp, &decodedDisplays)
	require.NoError(t, err)

	require.Len(t, decodedMessages, 3)
	assert.IsType(t, UserMessage{}, decodedMessages[0])
	assert.IsType(t, AssistantMessage{}, decodedMessages[1])
	assert.IsType(t, ToolMessage{}, decodedMessages[2])

	assert.Equal(t, "Hello from user", decodedMessages[0].(UserMessage).Content)
	assert.Equal(t, "Assistant response", decodedMessages[1].(AssistantMessage).Content)
	assert.Equal(t, "call-1", decodedMessages[2].(ToolMessage).ToolCallID)

	require.Len(t, decodedDisplays, 1)
	assert.IsType(t, ShellDisplay{}, decodedDisplays["call-1"])
	assert.Equal(t, "Listing files", decodedDisplays["call-1"].(ShellDisplay).Comment)
}
