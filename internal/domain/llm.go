package domain

import (
	"context"
	"encoding/json"
	"fmt"
)

// LLM is a self-contained language model instance.
type LLM interface {
	ID() string
	DisplayName() string
	ContextWindow() int
	ComputeTokens(ctx context.Context, msgs Messages) (int, error)
	Stream(ctx context.Context, msgs Messages, tools []Declaration) (Stream, error)
}

// LLMInfo is metadata for listing language models.
type LLMInfo struct {
	ID          string
	DisplayName string
}

// Provider represents an LLM service (e.g., Google, OpenAI).
// It acts as a factory and metadata source for authentication and model listing.
type Provider interface {
	ID() string
	SupportedAuthMethods() []AuthMethod
	ListLLMs() []LLMInfo
	GetLLM(ctx context.Context, cred *Credential, modelID string) (LLM, error)
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
type Message interface {
	Role() Role
}

type UserMessage struct {
	Content string `json:"content"`
}

func (UserMessage) Role() Role { return RoleUser }

func (m UserMessage) MarshalJSON() ([]byte, error) {
	type Alias UserMessage
	return json.Marshal(&struct {
		Role Role `json:"role"`
		*Alias
	}{
		Role:  RoleUser,
		Alias: (*Alias)(&m),
	})
}

type AssistantMessage struct {
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

func (AssistantMessage) Role() Role { return RoleAssistant }

func (m AssistantMessage) MarshalJSON() ([]byte, error) {
	type Alias AssistantMessage
	return json.Marshal(&struct {
		Role Role `json:"role"`
		*Alias
	}{
		Role:  RoleAssistant,
		Alias: (*Alias)(&m),
	})
}

type ToolMessage struct {
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	Content    string `json:"content"`
	ToolError  bool   `json:"tool_error,omitempty"`
}

func (ToolMessage) Role() Role { return RoleTool }

func (m ToolMessage) MarshalJSON() ([]byte, error) {
	type Alias ToolMessage
	return json.Marshal(&struct {
		Role Role `json:"role"`
		*Alias
	}{
		Role:  RoleTool,
		Alias: (*Alias)(&m),
	})
}

type SystemMessage struct {
	Content string `json:"content"`
}

func (SystemMessage) Role() Role { return RoleSystem }

func (m SystemMessage) MarshalJSON() ([]byte, error) {
	type Alias SystemMessage
	return json.Marshal(&struct {
		Role Role `json:"role"`
		*Alias
	}{
		Role:  RoleSystem,
		Alias: (*Alias)(&m),
	})
}

// Messages is a helper type for polymorphic JSON unmarshaling of Message slices.
type Messages []Message

func (m *Messages) UnmarshalJSON(data []byte) error {
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return err
	}

	*m = make(Messages, len(raws))
	for i, raw := range raws {
		var peek struct {
			Role Role `json:"role"`
		}
		if err := json.Unmarshal(raw, &peek); err != nil {
			return err
		}

		var msg Message
		switch peek.Role {
		case RoleUser:
			var u UserMessage
			if err := json.Unmarshal(raw, &u); err != nil {
				return err
			}
			msg = u
		case RoleAssistant:
			var a AssistantMessage
			if err := json.Unmarshal(raw, &a); err != nil {
				return err
			}
			msg = a
		case RoleTool:
			var t ToolMessage
			if err := json.Unmarshal(raw, &t); err != nil {
				return err
			}
			msg = t
		case RoleSystem:
			var s SystemMessage
			if err := json.Unmarshal(raw, &s); err != nil {
				return err
			}
			msg = s
		default:
			return fmt.Errorf("unknown message role: %s", peek.Role)
		}
		(*m)[i] = msg
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
