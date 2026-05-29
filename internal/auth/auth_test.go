package auth

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAuthMethodGitHubOAuth = "github_oauth"

// mockFileSystem implements auth.FileSystem for testing.
type mockFileSystem struct {
	files     map[string][]byte
	readCount int
}

func (m *mockFileSystem) ReadFile(name string) ([]byte, error) {
	m.readCount++
	if data, ok := m.files[name]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

func (m *mockFileSystem) WriteFile(name string, data []byte, _ os.FileMode) error {
	m.files[name] = data
	return nil
}

func (m *mockFileSystem) MkdirAll(_ string, _ os.FileMode) error {
	return nil
}

func TestAuth(t *testing.T) {
	mockFS := &mockFileSystem{files: make(map[string][]byte)}
	storePath := "/home/user/.config/iav/auth.json"

	t.Run("Get_NotFound", func(t *testing.T) {
		mgr := NewManager(mockFS, storePath)
		cred, err := mgr.Get("non-existent")
		assert.NoError(t, err)
		assert.Nil(t, cred)
	})

	t.Run("SetAndGet", func(t *testing.T) {
		mgr := NewManager(mockFS, storePath)
		cred := domain.Credential{
			Type:   domain.AuthMethodAPIKey,
			APIKey: "test-key",
		}
		err := mgr.Set(domain.ProviderGoogle, cred)
		assert.NoError(t, err)

		got, err := mgr.Get(domain.ProviderGoogle)
		assert.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, cred, *got)
	})

	t.Run("Set_OverwritesExisting", func(t *testing.T) {
		mgr := NewManager(mockFS, storePath)
		err := mgr.Set(domain.ProviderGoogle, domain.Credential{Type: domain.AuthMethodAPIKey, APIKey: "key1"})
		assert.NoError(t, err)

		newCred := domain.Credential{Type: domain.AuthMethodAPIKey, APIKey: "key2"}
		err = mgr.Set(domain.ProviderGoogle, newCred)
		assert.NoError(t, err)

		got, err := mgr.Get(domain.ProviderGoogle)
		assert.NoError(t, err)
		assert.Equal(t, "key2", got.APIKey)
	})

	t.Run("All_Empty", func(t *testing.T) {
		delete(mockFS.files, storePath)
		mgr := NewManager(mockFS, storePath)
		all, err := mgr.All()
		assert.NoError(t, err)
		assert.Empty(t, all)
	})

	t.Run("All_MultipleProviders", func(t *testing.T) {
		mgr := NewManager(mockFS, storePath)
		require.NoError(t, mgr.Set("p1", domain.Credential{Type: domain.AuthMethodAPIKey, APIKey: "k1"}))
		require.NoError(t, mgr.Set("p2", domain.Credential{Type: domain.AuthMethodAPIKey, APIKey: "k2"}))

		all, err := mgr.All()
		assert.NoError(t, err)
		assert.Len(t, all, 2)
		assert.Equal(t, "k1", all["p1"].APIKey)
		assert.Equal(t, "k2", all["p2"].APIKey)
	})

	t.Run("Remove", func(t *testing.T) {
		mgr := NewManager(mockFS, storePath)
		require.NoError(t, mgr.Set("rem", domain.Credential{Type: domain.AuthMethodAPIKey, APIKey: "val"}))
		err := mgr.Remove("rem")
		assert.NoError(t, err)

		got, err := mgr.Get("rem")
		assert.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("Remove_NonExistent", func(t *testing.T) {
		mgr := NewManager(mockFS, storePath)
		err := mgr.Remove("missing")
		assert.NoError(t, err)
	})

	t.Run("Get_CorruptFile", func(t *testing.T) {
		mockFS.files[storePath] = []byte("invalid json")
		mgr := NewManager(mockFS, storePath)
		_, err := mgr.Get("any")
		assert.Error(t, err)
	})
}

func TestManager_Caching(t *testing.T) {
	mockFS := &mockFileSystem{
		files: map[string][]byte{
			"/config/auth.json": []byte(`{"p1": {"type": "api_key", "api_key": "k1"}}`),
		},
	}
	mgr := NewManager(mockFS, "/config/auth.json")

	// 1. Initial Get should trigger a read
	got, err := mgr.Get("p1")
	assert.NoError(t, err)
	assert.NotNil(t, got)
	assert.Equal(t, 1, mockFS.readCount, "Should have read from disk once")

	// 2. Second Get should NOT trigger a read (cached)
	got2, err := mgr.Get("p1")
	assert.NoError(t, err)
	assert.NotNil(t, got2)
	assert.Equal(t, 1, mockFS.readCount, "Should NOT have read from disk again (cached)")

	// 3. Set should invalidate or update cache
	err = mgr.Set("p2", domain.Credential{Type: domain.AuthMethodAPIKey, APIKey: "k2"})
	assert.NoError(t, err)

	// 4. Get after Set might trigger a re-read if we invalidate, or use updated if we sync
	// For simplicity, let's assume we invalidate and re-read, OR just check it's consistent.
	got3, err := mgr.Get("p2")
	assert.NoError(t, err)
	assert.Equal(t, "k2", got3.APIKey)

	// 5. Remove should also keep cache consistent
	err = mgr.Remove("p1")
	assert.NoError(t, err)

	got4, err := mgr.Get("p1")
	assert.NoError(t, err)
	assert.Nil(t, got4)
}

type authMockProvider struct {
	id      string
	methods []domain.AuthMethod
}

func (p *authMockProvider) ID() string                                { return p.id }
func (p *authMockProvider) SupportedAuthMethods() []domain.AuthMethod { return p.methods }
func (p *authMockProvider) List() []domain.LLMInfo {
	return nil
}

func (p *authMockProvider) GetLLM(context.Context, *domain.Credential, domain.LLMInfo) (domain.LLM, error) {
	return nil, nil
}

func TestManager_GetWithFallback_RED(t *testing.T) {
	mockFS := &mockFileSystem{files: make(map[string][]byte)}
	storePath := "/home/user/.config/iav/auth.json"
	mgr := NewManager(mockFS, storePath)

	provider := &authMockProvider{
		id: "test-provider",
		methods: []domain.AuthMethod{
			domain.APIKeyAuthMethod{
				ID:   domain.AuthMethodAPIKey,
				Name: "API Key",
				Fields: []domain.AuthField{
					{ID: domain.AuthFieldAPIKey, EnvVar: "TEST_API_KEY"},
				},
			},
		},
	}

	t.Run("Priority 1: Disk Over Env", func(t *testing.T) {
		_ = os.Setenv("TEST_API_KEY", "env-value")
		defer func() { _ = os.Unsetenv("TEST_API_KEY") }()

		// Save to disk
		err := mgr.Set("test-provider", domain.Credential{Type: domain.AuthMethodAPIKey, APIKey: "disk-value"})
		require.NoError(t, err)

		got, err := mgr.GetWithFallback(provider)
		assert.NoError(t, err)
		assert.Equal(t, "disk-value", got.APIKey)
		assert.Equal(t, domain.AuthMethodAPIKey, got.Type)
	})

	t.Run("Priority 2: Fallback to Env", func(t *testing.T) {
		_ = os.Setenv("TEST_API_KEY", "env-value")
		defer func() { _ = os.Unsetenv("TEST_API_KEY") }()

		// Remove from disk
		err := mgr.Remove("test-provider")
		require.NoError(t, err)

		got, err := mgr.GetWithFallback(provider)
		assert.NoError(t, err)
		assert.Equal(t, "env-value", got.APIKey)
		assert.Equal(t, domain.AuthMethodEnv, got.Type)
	})

	t.Run("GetWithFallback returns stored OAuthToken", func(t *testing.T) {
		fs := &mockFileSystem{files: make(map[string][]byte)}
		p := &authMockProvider{
			id: domain.ProviderGitHub,
			methods: []domain.AuthMethod{
				domain.OAuthMethod{ID: testAuthMethodGitHubOAuth},
			},
		}

		creds := map[string]domain.Credential{
			domain.ProviderGitHub: {Type: testAuthMethodGitHubOAuth, OAuthToken: "gho_test"},
		}
		// #nosec G117
		data, _ := json.Marshal(creds)
		fs.files[storePath] = data

		m := NewManager(fs, storePath)
		cred, err := m.GetWithFallback(p)

		assert.NoError(t, err)
		assert.NotNil(t, cred)
		assert.Equal(t, "gho_test", cred.OAuthToken)
	})

	t.Run("Fallback to Env with Multiple Fields", func(t *testing.T) {
		pComplex := &authMockProvider{
			id: "complex",
			methods: []domain.AuthMethod{
				domain.APIKeyAuthMethod{
					ID:   "complex",
					Name: "Complex",
					Fields: []domain.AuthField{
						{ID: domain.AuthFieldAPIKey, EnvVar: "VAL1"},
					},
				},
			},
		}
		_ = os.Setenv("VAL1", "v1")
		defer func() { _ = os.Unsetenv("VAL1") }()

		got, err := mgr.GetWithFallback(pComplex)
		assert.NoError(t, err)
		assert.Equal(t, "v1", got.APIKey)
	})

	t.Run("Returns Nil if neither exists", func(t *testing.T) {
		_ = os.Unsetenv("TEST_API_KEY")
		err := mgr.Remove("test-provider")
		require.NoError(t, err)

		got, err := mgr.GetWithFallback(provider)
		assert.NoError(t, err)
		assert.Nil(t, got)
	})
}
