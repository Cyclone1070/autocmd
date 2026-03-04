package domain

import (
	"context"
	"encoding/json"
)

// LLM is a self-contained language model instance.
type LLM interface {
	ID() string
	DisplayName() string
	ContextWindow() int
	ComputeTokens(ctx context.Context, msgs []Message) (int, error)
	Stream(ctx context.Context, msgs []Message, tools []Declaration) (Stream, error)
}

// LLMInfo is metadata for listing language models.
type LLMInfo struct {
	ID          string
	DisplayName string
}

// Role represents message roles.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message represents a single turn in conversation history.
type Message struct {
	Role      Role       `json:"role"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"` // for assistant messages

	// Fields for tool response
	ToolCallID string `json:"tool_call_id,omitempty"` // for tool messages
	ToolName   string `json:"tool_name,omitempty"`    // for tools that match by name (Gemini)
	ToolError  bool   `json:"tool_error,omitempty"`   // True if the tool execution failed

	// NEW: Baked UI representation of tool calls
	ToolDisplays map[string]ToolDisplay `json:"tool_displays,omitempty"`
}

// UnmarshalJSON implements custom unmarshaling for Message to handle polymorphic ToolDisplays.
func (m *Message) UnmarshalJSON(data []byte) error {
	type Alias Message
	aux := &struct {
		ToolDisplays map[string]json.RawMessage `json:"tool_displays"`
		*Alias
	}{
		Alias: (*Alias)(m),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if len(aux.ToolDisplays) > 0 {
		m.ToolDisplays = make(map[string]ToolDisplay)
		for id, raw := range aux.ToolDisplays {
			var typeExtract struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(raw, &typeExtract); err != nil {
				continue
			}

			switch typeExtract.Type {
			case "string":
				var d StringDisplay
				if err := json.Unmarshal(raw, &d); err == nil {
					m.ToolDisplays[id] = d
				}
			case "diff":
				var d DiffDisplay
				if err := json.Unmarshal(raw, &d); err == nil {
					m.ToolDisplays[id] = d
				}
			case "shell":
				var d ShellDisplay
				if err := json.Unmarshal(raw, &d); err == nil {
					m.ToolDisplays[id] = d
				}
			}
		}
	}

	return nil
}

// Stream delivers response chunks from an LLM.
type Stream interface {
	// Next advances to the next chunk. Returns false when the stream ends or errors.
	Next() bool

	// Chunk returns the current chunk.
	Chunk() StreamChunk

	// Err returns any error encountered during streaming.
	Err() error
}

// StreamChunk is a piece of streamed response.
// Implemented by TextChunk, ToolCall.
type StreamChunk interface {
	isStreamChunk()
}

// TextChunk is a fragment of text response.
type TextChunk struct {
	Text string
}

func (TextChunk) isStreamChunk() {}

// ToolCall is the LLM's request to execute a tool.
// Implements StreamChunk.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (ToolCall) isStreamChunk() {}
