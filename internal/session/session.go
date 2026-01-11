package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Cyclone1070/iav/internal/provider"
)

// sessionInfoDTO is used for the .json file (metadata only).
type sessionInfoDTO struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Created int64  `json:"created"`
	Updated int64  `json:"updated"`
}

// sessionMessagesDTO is used for the .messages.json file.
type sessionMessagesDTO struct {
	Messages []provider.Message `json:"messages"`
}

// Session represents a conversation session with message history.
type Session struct {
	id         string
	name       string
	created    time.Time
	updated    time.Time
	messages   []provider.Message
	storageDir string
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

// MessageCount returns the number of messages without loading them all.
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

// Save persists the session to disk (both info and messages files).
func (s *Session) Save() error {
	s.updated = time.Now()

	// Write info file
	infoPath := filepath.Join(s.storageDir, s.id+".json")
	infoDTO := sessionInfoDTO{
		ID:      s.id,
		Name:    s.name,
		Created: s.created.UnixMilli(),
		Updated: s.updated.UnixMilli(),
	}
	infoData, err := json.MarshalIndent(infoDTO, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session info: %w", err)
	}
	if err := os.WriteFile(infoPath, infoData, 0644); err != nil {
		return fmt.Errorf("write session info: %w", err)
	}

	// Write messages file
	messagesPath := filepath.Join(s.storageDir, s.id+".messages.json")
	messagesDTO := sessionMessagesDTO{
		Messages: s.messages,
	}
	messagesData, err := json.MarshalIndent(messagesDTO, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session messages: %w", err)
	}
	if err := os.WriteFile(messagesPath, messagesData, 0644); err != nil {
		return fmt.Errorf("write session messages: %w", err)
	}

	return nil
}

// Clear removes all messages from the session but keeps the ID.
func (s *Session) Clear() {
	s.messages = []provider.Message{}
	s.updated = time.Now()
}
