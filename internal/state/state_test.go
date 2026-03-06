package state_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Cyclone1070/iav/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockFS implements state.FileSystem for testing
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

func (m *MockFS) WriteFile(path string, data []byte, perm os.FileMode) error {
	m.Files[path] = data
	return nil
}

func (m *MockFS) MkdirAll(path string, perm os.FileMode) error {
	m.Dirs = append(m.Dirs, path)
	return nil
}

func TestLoad_NoFile_ReturnsDefault(t *testing.T) {
	fs := &MockFS{Files: make(map[string][]byte)}
	loader := state.NewLoaderWithFS(fs)

	s, err := loader.Load()
	require.NoError(t, err)
	assert.Equal(t, "google/gemini-2.5-flash", s.Model) // Default model
	assert.Equal(t, "", s.CurrentSessionID)
}

func TestSave_PersistsToFile(t *testing.T) {
	fs := &MockFS{Files: make(map[string][]byte)}
	loader := state.NewLoaderWithFS(fs)

	s := &state.State{
		Model:            "custom-model",
		CurrentSessionID: "session-123",
	}

	err := loader.Save(s)
	require.NoError(t, err)

	// Verify file was written
	statePath := filepath.Join("/home/user", ".config", "iav", "state.json")
	assert.Contains(t, fs.Files, statePath)
	assert.Contains(t, string(fs.Files[statePath]), "custom-model")
	assert.Contains(t, string(fs.Files[statePath]), "session-123")
}

func TestLoad_ExistingFile_ReturnsContent(t *testing.T) {
	fs := &MockFS{Files: make(map[string][]byte)}
	statePath := filepath.Join("/home/user", ".config", "iav", "state.json")
	fs.Files[statePath] = []byte(`{"model": "saved-model", "current_session_id": "999"}`)
	
	loader := state.NewLoaderWithFS(fs)
	s, err := loader.Load()
	require.NoError(t, err)
	assert.Equal(t, "saved-model", s.Model)
	assert.Equal(t, "999", s.CurrentSessionID)
}
