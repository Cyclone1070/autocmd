package domain

import "time"

// Session represents a conversation session with message history.
// Session is a pure data object - use Store for persistence operations.
type Session struct {
	ID       string
	Name     string
	Created  time.Time
	Updated  time.Time
	Messages []Message
}

// SetName updates the session name.
func (s *Session) SetName(name string) {
	s.Name = name
	s.Updated = time.Now()
}

// MessageCount returns the number of messages.
func (s *Session) MessageCount() int {
	return len(s.Messages)
}

// GetMessages returns a copy of the messages slice for thread safety.
func (s *Session) GetMessages() []Message {
	result := make([]Message, len(s.Messages))
	copy(result, s.Messages)
	return result
}

// Add appends a message to the session.
func (s *Session) Add(msg Message) {
	s.Messages = append(s.Messages, msg)
	s.Updated = time.Now()
}

// Clear removes all messages from the session but keeps the ID.
func (s *Session) Clear() {
	s.Messages = []Message{}
	s.Updated = time.Now()
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
