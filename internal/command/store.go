// Package command provides a persistent store for saved bash commands.
package command

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Cyclone1070/autocmd/internal/domain"
)

var errInvalidName = fmt.Errorf("command name must not be empty")

// DefaultStorePath returns the default path for the commands JSON file.
func DefaultStorePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get user home dir: %w", err)
	}
	return filepath.Join(home, domain.ConfigBaseDir, domain.AppName, "commands.json"), nil
}

type fileSystem interface {
	WriteFileAtomic(path string, data []byte, perm os.FileMode) error
	ReadFile(path string) ([]byte, error)
	EnsureDirs(path string) error
	Remove(path string) error
	RemoveAll(path string) error
}

// Store persists saved commands as a single JSON file.
type Store struct {
	fs          fileSystem
	storagePath string
	mu          sync.RWMutex
	commands    map[string]*domain.SavedCommand
}

// NewStore creates a command store backed by a JSON file.
func NewStore(fs fileSystem, storagePath string) *Store {
	return &Store{
		fs:          fs,
		storagePath: storagePath,
		commands:    make(map[string]*domain.SavedCommand),
	}
}

func isEmptyOrWhitespace(s string) bool {
	return len(strings.TrimSpace(s)) == 0
}

func (s *Store) load() error {
	data, err := s.fs.ReadFile(s.storagePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read commands file: %w", err)
	}
	if len(data) == 0 {
		return nil
	}
	var cmds []*domain.SavedCommand
	if err := json.Unmarshal(data, &cmds); err != nil {
		// Corrupted file — reset
		s.commands = make(map[string]*domain.SavedCommand)
		return nil
	}
	s.commands = make(map[string]*domain.SavedCommand, len(cmds))
	for _, c := range cmds {
		s.commands[c.Name] = c
	}
	return nil
}

func (s *Store) persist() error {
	cmds := make([]*domain.SavedCommand, 0, len(s.commands))
	for _, c := range s.commands {
		cmds = append(cmds, c)
	}
	sort.Slice(cmds, func(i, j int) bool {
		return cmds[i].Name < cmds[j].Name
	})
	data, err := json.MarshalIndent(cmds, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal commands: %w", err)
	}
	if err := s.fs.EnsureDirs(dir(s.storagePath)); err != nil {
		return fmt.Errorf("ensure dirs: %w", err)
	}
	if err := s.fs.WriteFileAtomic(s.storagePath, data, domain.DefaultFilePerm); err != nil {
		return fmt.Errorf("write commands file: %w", err)
	}
	return nil
}

func dir(path string) string {
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[:idx]
	}
	return "."
}

// Save stores a command by name, creating or overwriting.
func (s *Store) Save(name, command, description string) error {
	if isEmptyOrWhitespace(name) {
		return errInvalidName
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.load(); err != nil {
		return err
	}

	now := time.Now().UnixMilli()
	existing, ok := s.commands[name]
	if ok {
		existing.Command = command
		existing.Description = description
		existing.UpdatedAt = now
	} else {
		s.commands[name] = &domain.SavedCommand{
			Name:        name,
			Command:     command,
			Description: description,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
	}

	return s.persist()
}

// Get returns a saved command by name.
func (s *Store) Get(name string) (*domain.SavedCommand, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := s.load(); err != nil {
		return nil, false
	}

	cmd, ok := s.commands[name]
	if !ok {
		return nil, false
	}
	return cmd, true
}

// List returns all saved commands sorted by name.
func (s *Store) List() []*domain.SavedCommand {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_ = s.load()

	cmds := make([]*domain.SavedCommand, 0, len(s.commands))
	for _, c := range s.commands {
		cmds = append(cmds, c)
	}
	sort.Slice(cmds, func(i, j int) bool {
		return cmds[i].Name < cmds[j].Name
	})
	return cmds
}

// Delete removes a saved command by name. Returns true if it existed.
func (s *Store) Delete(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.load(); err != nil {
		return false
	}

	if _, ok := s.commands[name]; !ok {
		return false
	}
	delete(s.commands, name)
	_ = s.persist()
	return true
}

// DeleteAll removes all saved commands.
func (s *Store) DeleteAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.commands = make(map[string]*domain.SavedCommand)
	_ = s.persist()
}
