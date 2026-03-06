package domain

import "time"

// Session represents a conversation session with message history.
// Session is a pure data object - use Store for persistence operations.
type Session struct {
	ID           string
	Name         string
	Created      time.Time
	Updated      time.Time
	Messages     Messages
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
