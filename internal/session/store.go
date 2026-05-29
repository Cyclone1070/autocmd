package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/randutil"
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
	TokenCount   int    `json:"tokenCount"`
	Created      int64  `json:"created"`
	Updated      int64  `json:"updated"`
	WorkingDir   string `json:"workingDir"`
	Active       bool   `json:"active"`
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

// GenerateName is a facade for the session.GenerateName function.
func (st *Store) GenerateName(ctx context.Context, llm domain.LLM, target string) (string, error) {
	return GenerateName(ctx, llm, target)
}

// sessionDir returns the directory path for a given session ID.
func (st *Store) sessionDir(id string) string {
	return filepath.Join(st.storageDir, id)
}

// Create creates a new session with a unique ID and saves it.
func (st *Store) Create(workingDir string) (*domain.Session, error) {
	if err := st.fs.EnsureDirs(st.storageDir); err != nil {
		return nil, fmt.Errorf("create storage dir: %w", err)
	}
	now := time.Now()
	var id string
	for i := range domain.MaxCollisionRetries {
		id = randutil.ShortID(domain.ShortIDLength)
		sessionDir := st.sessionDir(id)
		infoPath := filepath.Join(sessionDir, "metadata.json")
		_, err := st.fs.ReadFile(infoPath)
		if err != nil {
			if os.IsNotExist(err) {
				break
			}
			return nil, fmt.Errorf("check session existence: %w", err)
		}
		if i == domain.MaxCollisionRetries-1 {
			return nil, fmt.Errorf("failed to generate unique ID after 100 attempts")
		}
	}
	s := &domain.Session{
		SessionMetadata: domain.SessionMetadata{
			ID:         id,
			WorkingDir: workingDir,
			Created:    now,
			Updated:    now,
		},
		SessionMessages: domain.SessionMessages{
			Messages: []*schema.Message{},
		},
		SessionDisplays: domain.SessionDisplays{
			ToolDisplays: map[string]domain.ToolDisplay{},
		},
	}
	if err := st.SaveSession(s); err != nil {
		return nil, err
	}
	return s, nil
}

// GetMetadata reads only the metadata.json for a session.
func (st *Store) GetMetadata(id string) (*domain.SessionMetadata, error) {
	sessionDir := st.sessionDir(id)
	infoPath := filepath.Join(sessionDir, "metadata.json")

	data, err := st.fs.ReadFile(infoPath)
	if err != nil {
		return nil, fmt.Errorf("read session info: %w", err)
	}

	var dto sessionInfoDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return nil, fmt.Errorf("unmarshal session info: %w", err)
	}

	return &domain.SessionMetadata{
		ID:           dto.ID,
		Name:         dto.Name,
		MessageCount: dto.MessageCount,
		TokenCount:   dto.TokenCount,
		Created:      time.UnixMilli(dto.Created),
		Updated:      time.UnixMilli(dto.Updated),
		WorkingDir:   dto.WorkingDir,
		Active:       dto.Active,
	}, nil
}

// SaveMetadata persists only the metadata.json for a session.
func (st *Store) SaveMetadata(m *domain.SessionMetadata) error {
	sessionDir := st.sessionDir(m.ID)
	if err := st.fs.EnsureDirs(sessionDir); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}

	infoDTO := sessionInfoDTO{
		ID:           m.ID,
		Name:         m.Name,
		MessageCount: m.MessageCount,
		TokenCount:   m.TokenCount,
		Created:      m.Created.UnixMilli(),
		Updated:      m.Updated.UnixMilli(),
		Active:       m.Active,
		WorkingDir:   m.WorkingDir,
	}
	data, err := json.MarshalIndent(infoDTO, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session info: %w", err)
	}

	infoPath := filepath.Join(sessionDir, "metadata.json")
	if err := st.fs.WriteFileAtomic(infoPath, data, domain.DefaultFilePerm); err != nil {
		return fmt.Errorf("write session info: %w", err)
	}
	return nil
}

// getMessages reads only the messages.json for a session.
func (st *Store) getMessages(id string) (*domain.SessionMessages, error) {
	sessionDir := st.sessionDir(id)
	messagesPath := filepath.Join(sessionDir, "messages.json")

	data, err := st.fs.ReadFile(messagesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &domain.SessionMessages{Messages: []*schema.Message{}}, nil
		}
		return nil, fmt.Errorf("read session messages: %w", err)
	}

	var messages []*schema.Message
	if len(data) > 0 {
		if err := json.Unmarshal(data, &messages); err != nil {
			return nil, fmt.Errorf("unmarshal session messages: %w", err)
		}
	}
	if messages == nil {
		messages = []*schema.Message{}
	}

	return &domain.SessionMessages{Messages: messages}, nil
}

// saveMessages persists only the messages.json for a session.
func (st *Store) saveMessages(id string, msgs *domain.SessionMessages) error {
	sessionDir := st.sessionDir(id)
	if err := st.fs.EnsureDirs(sessionDir); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}

	messagesPath := filepath.Join(sessionDir, "messages.json")
	data, err := json.MarshalIndent(msgs.Messages, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session messages: %w", err)
	}
	if err := st.fs.WriteFileAtomic(messagesPath, data, domain.DefaultFilePerm); err != nil {
		return fmt.Errorf("write session messages: %w", err)
	}
	return nil
}

// getDisplays reads only the displays.json for a session.
func (st *Store) getDisplays(id string) (*domain.SessionDisplays, error) {
	sessionDir := st.sessionDir(id)
	displaysPath := filepath.Join(sessionDir, "displays.json")

	data, err := st.fs.ReadFile(displaysPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &domain.SessionDisplays{ToolDisplays: map[string]domain.ToolDisplay{}}, nil
		}
		return nil, fmt.Errorf("read session displays: %w", err)
	}

	var sd domain.SessionDisplays
	if len(data) > 0 {
		if err := json.Unmarshal(data, &sd); err != nil {
			return nil, fmt.Errorf("unmarshal session displays: %w", err)
		}
	}
	if sd.ToolDisplays == nil {
		sd.ToolDisplays = map[string]domain.ToolDisplay{}
	}

	return &sd, nil
}

// saveDisplays persists only the displays.json for a session.
func (st *Store) saveDisplays(id string, displays *domain.SessionDisplays) error {
	sessionDir := st.sessionDir(id)
	if err := st.fs.EnsureDirs(sessionDir); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}

	displaysPath := filepath.Join(sessionDir, "displays.json")
	data, err := json.MarshalIndent(displays.ToolDisplays, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session displays: %w", err)
	}
	if err := st.fs.WriteFileAtomic(displaysPath, data, domain.DefaultFilePerm); err != nil {
		return fmt.Errorf("write session displays: %w", err)
	}
	return nil
}

// GetSession loads a full session from disk by ID (metadata, messages, displays).
func (st *Store) GetSession(id string) (*domain.Session, error) {
	meta, err := st.GetMetadata(id)
	if err != nil {
		return nil, err
	}

	msgs, err := st.getMessages(id)
	if err != nil {
		return nil, err
	}

	displays, err := st.getDisplays(id)
	if err != nil {
		return nil, err
	}

	return &domain.Session{
		SessionMetadata: *meta,
		SessionMessages: *msgs,
		SessionDisplays: *displays,
	}, nil
}

// SaveSession persists a full session to disk (metadata, messages, displays).
func (st *Store) SaveSession(s *domain.Session) error {
	s.Updated = time.Now()
	s.MessageCount = len(s.Messages)
	s.TokenCount = s.TotalTokens()
	if err := st.SaveMetadata(&s.SessionMetadata); err != nil {
		return err
	}
	if err := st.saveMessages(s.ID, &s.SessionMessages); err != nil {
		return err
	}
	return st.saveDisplays(s.ID, &s.SessionDisplays)
}

// List returns metadata for all sessions sorted by update time (newest first).
func (st *Store) List() ([]domain.SessionMetadata, error) {
	entries, err := st.fs.ListDir(st.storageDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []domain.SessionMetadata{}, nil
		}
		return nil, fmt.Errorf("read storage dir: %w", err)
	}

	var summaries []domain.SessionMetadata

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionID := entry.Name()

		infoPath := filepath.Join(st.storageDir, sessionID, "metadata.json")
		infoData, err := st.fs.ReadFile(infoPath)
		if err != nil {
			continue
		}

		var infoDTO sessionInfoDTO
		if err := json.Unmarshal(infoData, &infoDTO); err != nil {
			continue
		}

		summaries = append(summaries, domain.SessionMetadata{
			ID:           infoDTO.ID,
			Name:         infoDTO.Name,
			MessageCount: infoDTO.MessageCount,
			TokenCount:   infoDTO.TokenCount,
			Created:      time.UnixMilli(infoDTO.Created),
			Updated:      time.UnixMilli(infoDTO.Updated),
			Active:       infoDTO.Active,
			WorkingDir:   infoDTO.WorkingDir,
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Updated.After(summaries[j].Updated)
	})

	return summaries, nil
}

// SetActive marks the session with the given ID as active, deactivating any
// other active sessions in the same working directory. The workingDir parameter
// is used for scoping — it does not modify the session's own WorkingDir field.
func (st *Store) SetActive(id, workingDir string) error {
	summaries, err := st.List()
	if err != nil {
		return err
	}

	var found bool
	for i := range summaries {
		s := &summaries[i]
		if s.ID == id {
			if s.Active {
				found = true
				continue
			}
			s.Active = true
			if err := st.SaveMetadata(s); err != nil {
				return err
			}
			found = true
			continue
		}
		if s.WorkingDir == workingDir && s.Active {
			s.Active = false
			if err := st.SaveMetadata(s); err != nil {
				return err
			}
		}
	}
	if !found {
		return fmt.Errorf("session %s not found", id)
	}
	return nil
}

// FindActiveForDir returns the active session metadata for the given working directory.
func (st *Store) FindActiveForDir(dir string) (*domain.SessionMetadata, error) {
	summaries, err := st.List()
	if err != nil {
		return nil, err
	}

	for _, s := range summaries {
		if s.WorkingDir == dir && s.Active {
			return &s, nil
		}
	}
	return nil, nil
}

// FindBlank returns the most recently updated session metadata that has no name and no messages.
func (st *Store) FindBlank(workingDir string) (*domain.SessionMetadata, error) {
	summaries, err := st.List()
	if err != nil {
		return nil, err
	}

	for _, s := range summaries {
		if s.Name == "" && s.MessageCount == 0 && s.WorkingDir == workingDir {
			return &s, nil
		}
	}
	return nil, nil
}

// Rename updates the name of a session.
func (st *Store) Rename(id, name string) error {
	meta, err := st.GetMetadata(id)
	if err != nil {
		return err
	}
	meta.Name = name
	return st.SaveMetadata(meta)
}

// Delete removes a session from disk by ID (entire session directory).
func (st *Store) Delete(id string) error {
	sessionDir := st.sessionDir(id)
	if err := st.fs.RemoveAll(sessionDir); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// LoadChecksums retrieves the stored checksums for a session.
func (st *Store) LoadChecksums(sessionID string) (map[string]string, error) {
	sessionDir := st.sessionDir(sessionID)
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
	sessionDir := st.sessionDir(sessionID)
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
