package domain

import (
	"context"
	"encoding/json"
)

// Provider is an LLM backend service (Gemini, OpenAI, etc).
type Provider interface {
	// Name returns the provider identifier (e.g., "gemini").
	Name() string

	// ListModels returns available models from this provider.
	ListModels(ctx context.Context) ([]Model, error)

	// Stream starts a streaming completion request.
	Stream(ctx context.Context, model string, msgs []Message, tools []Declaration) (Stream, error)
}

// Model represents a model available from a provider.
type Model struct {
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
