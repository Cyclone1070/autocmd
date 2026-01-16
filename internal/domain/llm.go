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
