package cmd

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/Cyclone1070/iav/internal/auth"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type helpersMockFS struct {
	files map[string][]byte
}

func (m *helpersMockFS) ReadFile(name string) ([]byte, error) {
	if data, ok := m.files[name]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

func (m *helpersMockFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	if m.files == nil {
		m.files = make(map[string][]byte)
	}
	m.files[name] = data
	return nil
}

func (m *helpersMockFS) MkdirAll(path string, perm os.FileMode) error {
	return nil
}

func TestResolveCredentialPriorityExtensive(t *testing.T) {
	mockFS := &helpersMockFS{files: make(map[string][]byte)}
	storePath := "/home/user/.config/iav/auth.json"

	t.Run("Priority: Config > Env (Happy Path)", func(t *testing.T) {
		mgr := auth.NewManager(mockFS, storePath)
		os.Setenv("GEMINI_API_KEY", "env-key")
		defer os.Unsetenv("GEMINI_API_KEY")

		configCred := domain.Credential{Type: domain.AuthMethodAPIKey, APIKey: "config-key"}
		data, _ := json.Marshal(map[string]domain.Credential{domain.ProviderGoogle: configCred})
		mockFS.files[storePath] = data

		got := resolveCredential(mgr, domain.ProviderGoogle)
		require.NotNil(t, got)
		assert.Equal(t, "config-key", got.APIKey)
	})

	t.Run("Priority: Fallback to Env if Config is empty/incomplete", func(t *testing.T) {
		mgr := auth.NewManager(mockFS, storePath)
		os.Setenv("GEMINI_API_KEY", "env-key")
		defer os.Unsetenv("GEMINI_API_KEY")

		// Zombie Config: entry exists but API key is empty
		configCred := domain.Credential{Type: domain.AuthMethodAPIKey, APIKey: ""}
		data, _ := json.Marshal(map[string]domain.Credential{domain.ProviderGoogle: configCred})
		mockFS.files[storePath] = data

		got := resolveCredential(mgr, domain.ProviderGoogle)
		require.NotNil(t, got)
		assert.Equal(t, "env-key", got.APIKey, "Should fallback to Env if Config entry is empty")
	})

	t.Run("Priority: Fallback to Env if Config file missing", func(t *testing.T) {
		mgr := auth.NewManager(mockFS, storePath)
		delete(mockFS.files, storePath)
		os.Setenv("GEMINI_API_KEY", "env-key")
		defer os.Unsetenv("GEMINI_API_KEY")

		got := resolveCredential(mgr, domain.ProviderGoogle)
		require.NotNil(t, got)
		assert.Equal(t, "env-key", got.APIKey)
	})

	t.Run("Priority: Nil if both empty", func(t *testing.T) {
		mgr := auth.NewManager(mockFS, storePath)
		delete(mockFS.files, storePath)
		os.Unsetenv("GEMINI_API_KEY")

		got := resolveCredential(mgr, domain.ProviderGoogle)
		assert.Nil(t, got)
	})
}
