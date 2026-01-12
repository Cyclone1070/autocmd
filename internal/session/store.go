package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/Cyclone1070/iav/internal/provider"
	"github.com/google/uuid"
)

// sessionInfoDTO is used for the .json file (metadata only).
type sessionInfoDTO struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	MessageCount int    `json:"messageCount"`
	Created      int64  `json:"created"`
	Updated      int64  `json:"updated"`
}

// sessionMessagesDTO is used for the .messages.json file.
type sessionMessagesDTO struct {
	Messages []provider.Message `json:"messages"`
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

// Store manages session creation, loading, saving, and listing.
type Store struct {
	storageDir string
}

// NewStore creates a new session store.
func NewStore(cfg *config.Config) *Store {
	return &Store{storageDir: cfg.Session.StorageDir}
}

// Create creates a new session with a unique ID and saves it.
func (st *Store) Create() (*Session, error) {
	if err := os.MkdirAll(st.storageDir, 0755); err != nil {
		return nil, fmt.Errorf("create storage dir: %w", err)
	}
	now := time.Now()
	s := &Session{
		id:       uuid.New().String(),
		name:     "",
		created:  now,
		updated:  now,
		messages: []provider.Message{},
	}
	if err := st.Save(s); err != nil {
		return nil, err
	}
	return s, nil
}

// Get loads a session from disk by ID (loads both info and messages).
func (st *Store) Get(id string) (*Session, error) {
	// Read info file
	infoPath := filepath.Join(st.storageDir, id+".json")
	infoData, err := os.ReadFile(infoPath)
	if err != nil {
		return nil, fmt.Errorf("read session info: %w", err)
	}

	var infoDTO sessionInfoDTO
	if err := json.Unmarshal(infoData, &infoDTO); err != nil {
		return nil, fmt.Errorf("unmarshal session info: %w", err)
	}

	// Read messages file
	messagesPath := filepath.Join(st.storageDir, id+".messages.json")
	var messages []provider.Message
	messagesData, err := os.ReadFile(messagesPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read session messages: %w", err)
		}
		// Messages file doesn't exist yet, use empty slice
		messages = []provider.Message{}
	} else {
		var messagesDTO sessionMessagesDTO
		if err := json.Unmarshal(messagesData, &messagesDTO); err != nil {
			return nil, fmt.Errorf("unmarshal session messages: %w", err)
		}
		messages = messagesDTO.Messages
	}

	return &Session{
		id:       infoDTO.ID,
		name:     infoDTO.Name,
		created:  time.UnixMilli(infoDTO.Created),
		updated:  time.UnixMilli(infoDTO.Updated),
		messages: messages,
	}, nil
}

// Save persists a session to disk (both info and messages files).
func (st *Store) Save(s *Session) error {
	// Update the updated timestamp
	s.updated = time.Now()

	// Write info file
	infoPath := filepath.Join(st.storageDir, s.id+".json")
	infoDTO := sessionInfoDTO{
		ID:           s.id,
		Name:         s.name,
		MessageCount: len(s.messages),
		Created:      s.created.UnixMilli(),
		Updated:      s.updated.UnixMilli(),
	}
	infoData, err := json.MarshalIndent(infoDTO, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session info: %w", err)
	}
	if err := fs.WriteFileAtomic(infoPath, infoData, 0644); err != nil {
		return fmt.Errorf("write session info: %w", err)
	}

	// Write messages file
	messagesPath := filepath.Join(st.storageDir, s.id+".messages.json")
	messagesDTO := sessionMessagesDTO{
		Messages: s.messages,
	}
	messagesData, err := json.MarshalIndent(messagesDTO, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session messages: %w", err)
	}
	if err := fs.WriteFileAtomic(messagesPath, messagesData, 0644); err != nil {
		return fmt.Errorf("write session messages: %w", err)
	}

	return nil
}

// List returns summaries of all sessions sorted by update time (newest first).
// Only loads metadata, NOT messages. Use Get() for full session data.
func (st *Store) List() ([]SessionSummary, error) {
	entries, err := os.ReadDir(st.storageDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []SessionSummary{}, nil
		}
		return nil, fmt.Errorf("read storage dir: %w", err)
	}

	var summaries []SessionSummary

	for _, entry := range entries {
		// Only look at .json files (not .messages.json)
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".messages.json") {
			continue
		}

		// Read only the info file
		infoPath := filepath.Join(st.storageDir, name)
		infoData, err := os.ReadFile(infoPath)
		if err != nil {
			continue // skip corrupted
		}

		var infoDTO sessionInfoDTO
		if err := json.Unmarshal(infoData, &infoDTO); err != nil {
			continue // skip corrupted
		}

		summaries = append(summaries, SessionSummary{
			ID:           infoDTO.ID,
			Name:         infoDTO.Name,
			MessageCount: infoDTO.MessageCount,
			Created:      time.UnixMilli(infoDTO.Created),
			Updated:      time.UnixMilli(infoDTO.Updated),
		})
	}

	// Sort by updated time, newest first
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Updated.After(summaries[j].Updated)
	})

	return summaries, nil
}

// Delete removes a session from disk by ID (both info and messages files).
func (st *Store) Delete(id string) error {
	infoPath := filepath.Join(st.storageDir, id+".json")
	messagesPath := filepath.Join(st.storageDir, id+".messages.json")

	// Remove both files, ignore "not exist" errors
	if err := os.Remove(infoPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(messagesPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
