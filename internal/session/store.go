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
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/google/uuid"
)

// fileSystem defines the filesystem operations needed by Store.
type fileSystem interface {
	WriteFileAtomic(path string, content []byte, perm os.FileMode) error
	EnsureDirs(path string) error
	ReadFile(path string) ([]byte, error)
	ListDir(path string) ([]os.DirEntry, error)
	Remove(path string) error
}

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
	Messages domain.Messages `json:"messages"`
}

// Store manages session creation, loading, saving, and listing.
type Store struct {
	storageDir string
	fs         fileSystem
}

// NewStore creates a new session store.
func NewStore(cfg *config.Config, fs fileSystem) *Store {
	return &Store{storageDir: cfg.Session.StorageDir, fs: fs}
}

// Create creates a new session with a unique ID and saves it.
func (st *Store) Create() (*domain.Session, error) {
	if err := st.fs.EnsureDirs(st.storageDir); err != nil {
		return nil, fmt.Errorf("create storage dir: %w", err)
	}
	now := time.Now()
	s := &domain.Session{
		ID:       uuid.New().String(),
		Name:     "",
		Created:  now,
		Updated:  now,
		Messages: []domain.Message{},
	}
	if err := st.Save(s); err != nil {
		return nil, err
	}
	return s, nil
}

// Get loads a session from disk by ID (loads both info and messages).
func (st *Store) Get(id string) (*domain.Session, error) {
	// Read info file
	infoPath := filepath.Join(st.storageDir, id+".json")
	infoData, err := st.fs.ReadFile(infoPath)
	if err != nil {
		return nil, fmt.Errorf("read session info: %w", err)
	}

	var infoDTO sessionInfoDTO
	if err := json.Unmarshal(infoData, &infoDTO); err != nil {
		return nil, fmt.Errorf("unmarshal session info: %w", err)
	}

	// Read messages file
	messagesPath := filepath.Join(st.storageDir, id+".messages.json")
	var messages []domain.Message
	messagesData, err := st.fs.ReadFile(messagesPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read session messages: %w", err)
		}
		// Messages file doesn't exist yet, use empty slice
		messages = []domain.Message{}
	} else {
		var messagesDTO sessionMessagesDTO
		if err := json.Unmarshal(messagesData, &messagesDTO); err != nil {
			return nil, fmt.Errorf("unmarshal session messages: %w", err)
		}
		messages = messagesDTO.Messages
	}

	return &domain.Session{
		ID:       infoDTO.ID,
		Name:     infoDTO.Name,
		Created:  time.UnixMilli(infoDTO.Created),
		Updated:  time.UnixMilli(infoDTO.Updated),
		Messages: messages,
	}, nil
}

// Save persists a session to disk (both info and messages files).
func (st *Store) Save(s *domain.Session) error {
	// Update the updated timestamp
	s.Updated = time.Now()

	// Write info file
	infoPath := filepath.Join(st.storageDir, s.ID+".json")
	infoDTO := sessionInfoDTO{
		ID:           s.ID,
		Name:         s.Name,
		MessageCount: len(s.Messages),
		Created:      s.Created.UnixMilli(),
		Updated:      s.Updated.UnixMilli(),
	}
	infoData, err := json.MarshalIndent(infoDTO, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session info: %w", err)
	}
	if err := st.fs.WriteFileAtomic(infoPath, infoData, 0644); err != nil {
		return fmt.Errorf("write session info: %w", err)
	}

	// Write messages file
	messagesPath := filepath.Join(st.storageDir, s.ID+".messages.json")
	messagesDTO := sessionMessagesDTO{
		Messages: domain.Messages(s.Messages),
	}
	messagesData, err := json.MarshalIndent(messagesDTO, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session messages: %w", err)
	}
	if err := st.fs.WriteFileAtomic(messagesPath, messagesData, 0644); err != nil {
		return fmt.Errorf("write session messages: %w", err)
	}

	return nil
}

// List returns summaries of all sessions sorted by update time (newest first).
// Only loads metadata, NOT messages. Use Get() for full session data.
func (st *Store) List() ([]domain.SessionSummary, error) {
	entries, err := st.fs.ListDir(st.storageDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []domain.SessionSummary{}, nil
		}
		return nil, fmt.Errorf("read storage dir: %w", err)
	}

	var summaries []domain.SessionSummary

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
		infoData, err := st.fs.ReadFile(infoPath)
		if err != nil {
			continue // skip corrupted
		}

		var infoDTO sessionInfoDTO
		if err := json.Unmarshal(infoData, &infoDTO); err != nil {
			continue // skip corrupted
		}

		summaries = append(summaries, domain.SessionSummary{
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

// FindBlank returns the most recently updated session that has no name and no messages.
// Returns nil if no blank session is found.
func (st *Store) FindBlank() (*domain.SessionSummary, error) {
	summaries, err := st.List()
	if err != nil {
		return nil, err
	}

	for _, s := range summaries {
		if s.Name == "" && s.MessageCount == 0 {
			return &s, nil
		}
	}
	return nil, nil
}

// Rename updates the name of a session.
func (st *Store) Rename(id, name string) error {
	s, err := st.Get(id)
	if err != nil {
		return err
	}
	s.Name = name
	return st.Save(s)
}

// Delete removes a session from disk by ID (both info and messages files).
func (st *Store) Delete(id string) error {
	infoPath := filepath.Join(st.storageDir, id+".json")
	messagesPath := filepath.Join(st.storageDir, id+".messages.json")

	// Remove both files, ignore "not exist" errors
	if err := st.fs.Remove(infoPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := st.fs.Remove(messagesPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
