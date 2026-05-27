package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
)

// fileSystem defines the filesystem operations needed by Store.
type fileSystem interface {
	WriteFileAtomic(path string, content []byte, perm os.FileMode) error
	EnsureDirs(path string) error
	ReadFile(path string) ([]byte, error)
	ListDir(path string) ([]os.DirEntry, error)
	Remove(path string) error
	RemoveAll(path string) error
}

// sessionInfoDTO is used for the .json file (metadata only).
type sessionInfoDTO struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	MessageCount int    `json:"messageCount"`
	Created      int64  `json:"created"`
	Updated      int64  `json:"updated"`
	WorkingDir   string `json:"workingDir"`
}

// Store manages session creation, loading, saving, and listing.
type Store struct {
	fs         fileSystem
	storageDir string
}

// DefaultStorageDir returns the default session storage path (~/.config/iav/sessions).
func DefaultStorageDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get user home dir: %w", err)
	}
	return filepath.Join(home, domain.ConfigBaseDir, domain.AppName, "sessions"), nil
}

// NewStore creates a new session store.
func NewStore(fs fileSystem, storageDir string) *Store {
	return &Store{
		storageDir: storageDir,
		fs:         fs,
	}
}

const hexRatio = 2

func randomShortID(length int) string {
	bytes := make([]byte, length/hexRatio)
	if _, err := rand.Read(bytes); err != nil {
		panic(fmt.Sprintf("failed to generate random ID: %v", err))
	}
	return hex.EncodeToString(bytes)
}

// GenerateName is a facade for the session.GenerateName function.
func (st *Store) GenerateName(ctx context.Context, llm domain.LLM, target string) (string, error) {
	return GenerateName(ctx, llm, target)
}

// Create creates a new session with a unique ID and saves it.
func (st *Store) Create() (*domain.Session, error) {
	if err := st.fs.EnsureDirs(st.storageDir); err != nil {
		return nil, fmt.Errorf("create storage dir: %w", err)
	}
	now := time.Now()
	var id string
	for i := range domain.MaxCollisionRetries {
		id = randomShortID(domain.ShortIDLength)
		sessionDir := filepath.Join(st.storageDir, id)
		infoPath := filepath.Join(sessionDir, "metadata.json")
		_, err := st.fs.ReadFile(infoPath)
		if err != nil {
			if os.IsNotExist(err) {
				break // Found a unique ID
			}
			return nil, fmt.Errorf("check session existence: %w", err)
		}
		if i == domain.MaxCollisionRetries-1 {
			return nil, fmt.Errorf("failed to generate unique ID after 100 attempts")
		}
	}
	s := &domain.Session{
		ID:       id,
		Name:     "",
		Created:  now,
		Updated:  now,
		Messages: []*schema.Message{},
	}
	if err := st.Save(s); err != nil {
		return nil, err
	}
	return s, nil
}

// Get loads a session from disk by ID (loads both info and messages).
func (st *Store) Get(id string) (*domain.Session, error) {
	sessionDir := filepath.Join(st.storageDir, id)

	// Read info file
	infoPath := filepath.Join(sessionDir, "metadata.json")
	infoData, err := st.fs.ReadFile(infoPath)
	if err != nil {
		return nil, fmt.Errorf("read session info: %w", err)
	}

	var infoDTO sessionInfoDTO
	if err := json.Unmarshal(infoData, &infoDTO); err != nil {
		return nil, fmt.Errorf("unmarshal session info: %w", err)
	}

	// Read messages file
	messagesPath := filepath.Join(sessionDir, "messages.json")
	messagesData, err := st.fs.ReadFile(messagesPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read session messages: %w", err)
	}

	var messages []*schema.Message
	if len(messagesData) > 0 {
		if err := json.Unmarshal(messagesData, &messages); err != nil {
			return nil, fmt.Errorf("unmarshal session messages: %w", err)
		}
	} else {
		messages = []*schema.Message{}
	}

	// Read displays file
	displaysPath := filepath.Join(sessionDir, "displays.json")
	displaysData, err := st.fs.ReadFile(displaysPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read session displays: %w", err)
	}

	displays := make(domain.ToolDisplays)
	if len(displaysData) > 0 {
		if err := json.Unmarshal(displaysData, &displays); err != nil {
			return nil, fmt.Errorf("unmarshal session displays: %w", err)
		}
	}

	return &domain.Session{
		ID:           infoDTO.ID,
		Name:         infoDTO.Name,
		Created:      time.UnixMilli(infoDTO.Created),
		Updated:      time.UnixMilli(infoDTO.Updated),
		Messages:     messages,
		ToolDisplays: displays,
		WorkingDir:   infoDTO.WorkingDir,
	}, nil
}

// Save persists a session to disk (both info and messages files).
func (st *Store) Save(s *domain.Session) error {
	sessionDir := filepath.Join(st.storageDir, s.ID)
	if err := st.fs.EnsureDirs(sessionDir); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}

	// Update the updated timestamp
	s.Updated = time.Now()

	// Write info file
	infoPath := filepath.Join(sessionDir, "metadata.json")
	infoDTO := sessionInfoDTO{
		ID:           s.ID,
		Name:         s.Name,
		MessageCount: len(s.Messages),
		Created:      s.Created.UnixMilli(),
		Updated:      s.Updated.UnixMilli(),
		WorkingDir:   s.WorkingDir,
	}
	infoData, err := json.MarshalIndent(infoDTO, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session info: %w", err)
	}
	if err := st.fs.WriteFileAtomic(infoPath, infoData, domain.DefaultFilePerm); err != nil {
		return fmt.Errorf("write session info: %w", err)
	}

	// Write messages file
	messagesPath := filepath.Join(sessionDir, "messages.json")
	messagesData, err := json.MarshalIndent(s.Messages, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session messages: %w", err)
	}
	if err := st.fs.WriteFileAtomic(messagesPath, messagesData, domain.DefaultFilePerm); err != nil {
		return fmt.Errorf("write session messages: %w", err)
	}

	// Write displays file
	displaysPath := filepath.Join(sessionDir, "displays.json")
	displaysData, err := json.MarshalIndent(s.ToolDisplays, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session displays: %w", err)
	}
	if err := st.fs.WriteFileAtomic(displaysPath, displaysData, domain.DefaultFilePerm); err != nil {
		return fmt.Errorf("write session displays: %w", err)
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
		if !entry.IsDir() {
			continue
		}
		sessionID := entry.Name()

		// Read only the info file
		infoPath := filepath.Join(st.storageDir, sessionID, "metadata.json")
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
			WorkingDir:   infoDTO.WorkingDir,
		})
	}

	// Sort by updated time, newest first
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Updated.After(summaries[j].Updated)
	})

	return summaries, nil
}

// ListForDir returns summaries of sessions scoped to a working directory, sorted by update time (newest first).
func (st *Store) ListForDir(dir string) ([]domain.SessionSummary, error) {
	summaries, err := st.List()
	if err != nil {
		return nil, err
	}

	var filtered []domain.SessionSummary
	for _, s := range summaries {
		if s.WorkingDir == dir {
			filtered = append(filtered, s)
		}
	}
	return filtered, nil
}

// FindLatestForDir returns the most recently updated session for a given working directory.
// Returns nil if no session is found.
func (st *Store) FindLatestForDir(dir string) (*domain.SessionSummary, error) {
	summaries, err := st.ListForDir(dir)
	if err != nil {
		return nil, err
	}
	if len(summaries) == 0 {
		return nil, nil
	}
	return &summaries[0], nil
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

// Delete removes a session from disk by ID (entire session directory).
func (st *Store) Delete(id string) error {
	sessionDir := filepath.Join(st.storageDir, id)

	if err := st.fs.RemoveAll(sessionDir); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// LoadChecksums retrieves the stored checksums for a session.
func (st *Store) LoadChecksums(sessionID string) (map[string]string, error) {
	sessionDir := filepath.Join(st.storageDir, sessionID)
	checksumsPath := filepath.Join(sessionDir, "checksums.json")

	data, err := st.fs.ReadFile(checksumsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}
		return nil, fmt.Errorf("read checksums: %w", err)
	}

	var checksums map[string]string
	if err := json.Unmarshal(data, &checksums); err != nil {
		return nil, fmt.Errorf("unmarshal checksums: %w", err)
	}

	return checksums, nil
}

// SaveChecksums persists the checksums for a session.
func (st *Store) SaveChecksums(sessionID string, checksums map[string]string) error {
	sessionDir := filepath.Join(st.storageDir, sessionID)
	if err := st.fs.EnsureDirs(sessionDir); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}

	checksumsPath := filepath.Join(sessionDir, "checksums.json")
	if len(checksums) == 0 {
		if err := st.fs.Remove(checksumsPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove checksums file: %w", err)
		}
		return nil
	}

	data, err := json.MarshalIndent(checksums, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal checksums: %w", err)
	}

	if err := st.fs.WriteFileAtomic(checksumsPath, data, domain.DefaultFilePerm); err != nil {
		return fmt.Errorf("write checksums: %w", err)
	}

	return nil
}
