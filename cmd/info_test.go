package cmd

import (
	"bytes"
	"os"
	"testing"

	"github.com/Cyclone1070/iav/internal/auth"
	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type infoMockFS struct {
	files map[string][]byte
}

func (m *infoMockFS) ReadFile(name string) ([]byte, error) {
	if d, ok := m.files[name]; ok {
		return d, nil
	}
	return nil, os.ErrNotExist
}
func (m *infoMockFS) WriteFile(name string, d []byte, p os.FileMode) error {
	if m.files == nil {
		m.files = make(map[string][]byte)
	}
	m.files[name] = d
	return nil
}
func (m *infoMockFS) MkdirAll(p string, perm os.FileMode) error { return nil }
func (m *infoMockFS) UserHomeDir() (string, error)         { return "/home/user", nil }
func (m *infoMockFS) ListDir(path string) ([]os.DirEntry, error) { return nil, nil }
func (m *infoMockFS) Remove(path string) error               { return nil }
func (m *infoMockFS) Stat(path string) (os.FileInfo, error) { return nil, nil }
func (m *infoMockFS) WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	return m.WriteFile(path, data, perm)
}
func (m *infoMockFS) EnsureDirs(path string) error { return nil }

func TestInfoGracefulness(t *testing.T) {
	cfg := config.DefaultConfig()
	s := &state.State{}
	s.SetModel("google/gemini-2.0-flash")
	mockFS := &infoMockFS{}
	authMgr := auth.NewManager(mockFS, "/tmp/auth.json")

	// Clear keys
	os.Unsetenv("GEMINI_API_KEY")

	// Execute info logic
	b := &bytes.Buffer{}
	infoCmd.SetOut(b)

	err := runInfo(infoCmd, mockFS, cfg, s, authMgr)

	// Success condition: No error even if LLM fails
	assert.NoError(t, err)

	output := b.String()
	assert.Contains(t, output, "Model:", "Output SHOULD contain 'Model:' label if set in state")
	assert.NotContains(t, output, "Context Window:", "Output should NOT contain 'Context Window' if initialization fails")
}

func TestInfoAuthAwareness(t *testing.T) {
	cfg := config.DefaultConfig()

	t.Run("Scenario: No Auth - Omit Model and Authorized Providers", func(t *testing.T) {
		mockFS := &infoMockFS{files: make(map[string][]byte)}
		authMgr := auth.NewManager(mockFS, "/tmp/auth.json")
		os.Unsetenv("GEMINI_API_KEY")

		s := &state.State{}
		s.SetModel("google/gemini-2.0-flash")
		b := &bytes.Buffer{}
		infoCmd.SetOut(b)

		err := runInfo(infoCmd, mockFS, cfg, s, authMgr)
		require.NoError(t, err)

		output := b.String()
		assert.Contains(t, output, "Model:", "Should show Model section if set in state")
		assert.NotContains(t, output, "Authorized Providers:", "Should NOT show Authorized Providers if none")
	})

	t.Run("Scenario: Partial Auth - Show only authed", func(t *testing.T) {
		mockFS := &infoMockFS{files: make(map[string][]byte)}
		authMgr := auth.NewManager(mockFS, "/tmp/auth.json")
		os.Setenv("GEMINI_API_KEY", "test-key")
		defer os.Unsetenv("GEMINI_API_KEY")

		s := &state.State{}
		s.SetModel("google/gemini-2.0-flash")
		b := &bytes.Buffer{}
		infoCmd.SetOut(b)

		err := runInfo(infoCmd, mockFS, cfg, s, authMgr)
		require.NoError(t, err)

		output := b.String()
		assert.Contains(t, output, "Model:", "Should show Model section if authed")
		assert.Contains(t, output, "google/gemini-2.0-flash")
		assert.Contains(t, output, "Authorized Providers:", "Should show Authorized Providers")
		assert.Contains(t, output, "google (env)", "Should show (env) tag for environment variables")
	})

	t.Run("Scenario: Current Model Not Authed, but others are", func(t *testing.T) {
		mockFS := &infoMockFS{files: make(map[string][]byte)}
		authMgr := auth.NewManager(mockFS, "/tmp/auth.json")
		os.Unsetenv("GEMINI_API_KEY")
		// Auth for 'google' via mockFS
		mockFS.files["/tmp/auth.json"] = []byte(`{"google": {"type": "api_key", "api_key": "google-key"}}`)

		s := &state.State{}
		s.SetModel("other/model")
		b := &bytes.Buffer{}
		infoCmd.SetOut(b)

		err := runInfo(infoCmd, mockFS, cfg, s, authMgr)
		require.NoError(t, err)

		output := b.String()
		assert.Contains(t, output, "Model:", "Should show Model section even if current model is not authed")
		assert.Contains(t, output, "Authorized Providers:")
		assert.Contains(t, output, "google (api_key)", "Should show (api_key) tag for config-based auth")
	})
}
