package session

import (
	"time"

	"github.com/Cyclone1070/iav/internal/provider"
)

// Session represents a conversation session with message history.
// Session is a pure data object - use Store for persistence operations.
type Session struct {
	id       string
	name     string
	created  time.Time
	updated  time.Time
	messages []provider.Message
}

// ID returns the session identifier.
func (s *Session) ID() string {
	return s.id
}

// Name returns the session name.
func (s *Session) Name() string {
	return s.name
}

// SetName updates the session name.
func (s *Session) SetName(name string) {
	s.name = name
	s.updated = time.Now()
}

// Created returns the session creation time.
func (s *Session) Created() time.Time {
	return s.created
}

// Updated returns the session last update time.
func (s *Session) Updated() time.Time {
	return s.updated
}

// MessageCount returns the number of messages.
func (s *Session) MessageCount() int {
	return len(s.messages)
}

// Messages returns a copy of the messages slice for thread safety.
func (s *Session) Messages() []provider.Message {
	result := make([]provider.Message, len(s.messages))
	copy(result, s.messages)
	return result
}

// Add appends a message to the session.
func (s *Session) Add(msg provider.Message) {
	s.messages = append(s.messages, msg)
	s.updated = time.Now()
}

// Clear removes all messages from the session but keeps the ID.
func (s *Session) Clear() {
	s.messages = []provider.Message{}
	s.updated = time.Now()
}
