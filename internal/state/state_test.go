package state_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockFS implements state.FileSystem for testing.
type MockFS struct {
	Files map[string][]byte
	Dirs  []string
}

func (m *MockFS) UserHomeDir() (string, error) {
	return "/home/user", nil
}

func (m *MockFS) ReadFile(path string) ([]byte, error) {
	data, ok := m.Files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}

func (m *MockFS) WriteFile(path string, data []byte, _ os.FileMode) error {
	m.Files[path] = data
	return nil
}

func (m *MockFS) MkdirAll(path string, _ os.FileMode) error {
	m.Dirs = append(m.Dirs, path)
	return nil
}

func TestLoad_NoFile_ReturnsDefault(t *testing.T) {
	fs := &MockFS{Files: make(map[string][]byte)}
	mgr := state.NewManager(fs)

	s, err := mgr.Load()
	require.NoError(t, err)
	assert.Equal(t, "", s.Model()) // Default model should be empty
}

func TestSave_PersistsToFile(t *testing.T) {
	fs := &MockFS{Files: make(map[string][]byte)}
	mgr := state.NewManager(fs)

	s, _ := mgr.Load()
	s.SetModel("custom-model")

	err := mgr.Save(s)
	require.NoError(t, err)

	// Verify file was written
	statePath := filepath.Join("/home/user", domain.ConfigBaseDir, domain.AppName, "state.json")
	assert.Contains(t, fs.Files, statePath)
	assert.Contains(t, string(fs.Files[statePath]), "custom-model")
}

func TestLoad_ExistingFile_ReturnsContent(t *testing.T) {
	fs := &MockFS{Files: make(map[string][]byte)}
	statePath := filepath.Join("/home/user", domain.ConfigBaseDir, domain.AppName, "state.json")
	fs.Files[statePath] = []byte(`{"model": "saved-model"}`)

	mgr := state.NewManager(fs)
	s, err := mgr.Load()
	require.NoError(t, err)
	assert.Equal(t, "saved-model", s.Model())
}

