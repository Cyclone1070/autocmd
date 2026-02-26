package auth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuth(t *testing.T) {
	// Setup temp auth file
	tmpDir, err := os.MkdirTemp("", "iav-auth-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	origStorePath := storePath
	storePath = func() string {
		return filepath.Join(tmpDir, "auth.json")
	}
	defer func() { storePath = origStorePath }()

	t.Run("Get_NotFound", func(t *testing.T) {
		cred, err := Get("non-existent")
		assert.NoError(t, err)
		assert.Nil(t, cred)
	})

	t.Run("SetAndGet", func(t *testing.T) {
		cred := domain.Credential{
			Type:   "api_key",
			APIKey: "test-key",
		}
		err := Set("google", cred)
		assert.NoError(t, err)

		got, err := Get("google")
		assert.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, cred, *got)
	})

	t.Run("Set_OverwritesExisting", func(t *testing.T) {
		err := Set("google", domain.Credential{Type: "api_key", APIKey: "key1"})
		assert.NoError(t, err)

		newCred := domain.Credential{Type: "api_key", APIKey: "key2"}
		err = Set("google", newCred)
		assert.NoError(t, err)

		got, err := Get("google")
		assert.NoError(t, err)
		assert.Equal(t, "key2", got.APIKey)
	})

	t.Run("All_Empty", func(t *testing.T) {
		// Clear file for this test
		_ = os.Remove(storePath())
		all, err := All()
		assert.NoError(t, err)
		assert.Empty(t, all)
	})

	t.Run("All_MultipleProviders", func(t *testing.T) {
		require.NoError(t, Set("p1", domain.Credential{Type: "api_key", APIKey: "k1"}))
		require.NoError(t, Set("p2", domain.Credential{Type: "api_key", APIKey: "k2"}))

		all, err := All()
		assert.NoError(t, err)
		assert.Len(t, all, 2)
		assert.Equal(t, "k1", all["p1"].APIKey)
		assert.Equal(t, "k2", all["p2"].APIKey)
	})

	t.Run("Remove", func(t *testing.T) {
		require.NoError(t, Set("rem", domain.Credential{Type: "api_key", APIKey: "val"}))
		err := Remove("rem")
		assert.NoError(t, err)

		got, err := Get("rem")
		assert.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("Remove_NonExistent", func(t *testing.T) {
		err := Remove("missing")
		assert.NoError(t, err)
	})

	t.Run("Get_CorruptFile", func(t *testing.T) {
		require.NoError(t, os.WriteFile(storePath(), []byte("invalid json"), 0600))
		_, err := Get("any")
		assert.Error(t, err)
	})
}
