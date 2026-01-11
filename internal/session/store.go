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
	"github.com/Cyclone1070/iav/internal/provider"
	"github.com/google/uuid"
)

// Store manages session creation, loading, and listing.
type Store struct {
	storageDir string
}

// NewStore creates a new session store.
func NewStore(cfg *config.Config) *Store {
	return &Store{storageDir: cfg.Session.StorageDir}
}

// Create creates a new session with a unique ID.
func (st *Store) Create() (*Session, error) {
	if err := os.MkdirAll(st.storageDir, 0755); err != nil {
		return nil, fmt.Errorf("create storage dir: %w", err)
	}
	now := time.Now()
	s := &Session{
		id:         uuid.New().String(),
		name:       "",
		created:    now,
		updated:    now,
		messages:   []provider.Message{},
		storageDir: st.storageDir,
	}
	if err := s.Save(); err != nil {
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
		id:         infoDTO.ID,
		name:       infoDTO.Name,
		created:    time.UnixMilli(infoDTO.Created),
		updated:    time.UnixMilli(infoDTO.Updated),
		messages:   messages,
		storageDir: st.storageDir,
	}, nil
}

// List returns all sessions sorted by update time (newest first).
// Only loads info files, NOT messages (efficient for large sessions).
func (st *Store) List() ([]*Session, error) {
	entries, err := os.ReadDir(st.storageDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Session{}, nil
		}
		return nil, fmt.Errorf("read storage dir: %w", err)
	}

	type sessionMeta struct {
		session *Session
		updated time.Time
	}
	var metas []sessionMeta

	for _, entry := range entries {
		// Only look at .json files (not .messages.json)
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".messages.json") {
			continue
		}

		id := name[:len(name)-5] // strip .json

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

		// Create session without loading messages
		s := &Session{
			id:         infoDTO.ID,
			name:       infoDTO.Name,
			created:    time.UnixMilli(infoDTO.Created),
			updated:    time.UnixMilli(infoDTO.Updated),
			messages:   nil, // NOT loaded
			storageDir: st.storageDir,
		}
		metas = append(metas, sessionMeta{session: s, updated: s.updated})
		_ = id // silence unused warning
	}

	// Sort by updated time, newest first
	sort.Slice(metas, func(i, j int) bool {
		return metas[i].updated.After(metas[j].updated)
	})

	sessions := make([]*Session, len(metas))
	for i, m := range metas {
		sessions[i] = m.session
	}
	return sessions, nil
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
