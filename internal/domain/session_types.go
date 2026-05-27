package domain

// Session aggregate and summary types.

import (
	"slices"
	"time"

	"github.com/cloudwego/eino/schema"
)

// Session represents a conversation session with message history.
// Session is a pure data object - use Store for persistence operations.
type Session struct {
	Created      time.Time
	Updated      time.Time
	ToolDisplays ToolDisplays
	ID           string
	Name         string
	Messages     []*schema.Message
	WorkingDir   string
}

// SessionSummary contains metadata about a session without messages.
// Returned by List() for efficient browsing without loading full sessions.
type SessionSummary struct {
	Created      time.Time
	Updated      time.Time
	ID           string
	Name         string
	MessageCount int
	WorkingDir   string
}

// TotalTokens returns the factual total tokens in the session as of the last model response.
// It ignores any uncommitted user messages that haven't been processed by the LLM yet.
func (s *Session) TotalTokens() int {
	for i := range slices.Backward(s.Messages) {
		m := s.Messages[i]
		if m.Role == schema.Assistant && m.ResponseMeta != nil && m.ResponseMeta.Usage != nil {
			return m.ResponseMeta.Usage.TotalTokens
		}
	}
	return 0
}
