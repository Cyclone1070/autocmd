package domain

// Session aggregate and summary types.

import (
	"time"

	"github.com/cloudwego/eino/schema"
)

// Session represents a conversation session with message history.
// Session is a pure data object - use Store for persistence operations.
type Session struct {
	ID           string
	Name         string
	Created      time.Time
	Updated      time.Time
	Messages     []*schema.Message
	ToolDisplays ToolDisplays
}

// SessionSummary contains metadata about a session without messages.
// Returned by List() for efficient browsing without loading full sessions.
type SessionSummary struct {
	ID           string
	Name         string
	MessageCount int
	Created      time.Time
	Updated      time.Time
}

// TotalTokens returns the factual total tokens in the session as of the last model response.
// It ignores any uncommitted user messages that haven't been processed by the LLM yet.
func (s *Session) TotalTokens() int {
	for i := len(s.Messages) - 1; i >= 0; i-- {
		m := s.Messages[i]
		if m.Role == schema.Assistant && m.ResponseMeta != nil && m.ResponseMeta.Usage != nil {
			return m.ResponseMeta.Usage.TotalTokens
		}
	}
	return 0
}
