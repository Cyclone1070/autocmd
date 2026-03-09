package cmd

import (
	"context"
	"os"
	"testing"

	"github.com/Cyclone1070/iav/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type modelMockFS struct {
	files map[string][]byte
}

func (m *modelMockFS) ReadFile(name string) ([]byte, error) {
	if data, ok := m.files[name]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

func (m *modelMockFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	if m.files == nil {
		m.files = make(map[string][]byte)
	}
	m.files[name] = data
	return nil
}

func (m *modelMockFS) MkdirAll(path string, perm os.FileMode) error {
	return nil
}

func TestModelAuthAwareness(t *testing.T) {
	mockFS := &modelMockFS{files: make(map[string][]byte)}
	storePath := "/home/user/.config/iav/auth.json"
	mgr := auth.NewManager(mockFS, storePath)

	t.Run("Scenario: No Auth - Should show error or empty list", func(t *testing.T) {
		os.Unsetenv("GEMINI_API_KEY")
		delete(mockFS.files, storePath)

		registry := buildLLMRegistry(mgr)
		models, err := registry.List(context.Background())
		require.NoError(t, err)

		assert.Empty(t, models, "Should return no models if no providers are authed")
	})

	t.Run("Scenario: Partial Auth - Only show models from authed provider", func(t *testing.T) {
		// Mock auth for 'google'
		os.Setenv("GEMINI_API_KEY", "test-key")
		defer os.Unsetenv("GEMINI_API_KEY")

		registry := buildLLMRegistry(mgr)
		models, err := registry.List(context.Background())
		require.NoError(t, err)

		assert.NotEmpty(t, models)
		for _, m := range models {
			assert.Contains(t, m.ID, "google/", "All listed models should belong to an authed provider (google)")
		}
	})
}
