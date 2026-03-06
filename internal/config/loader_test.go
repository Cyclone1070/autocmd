package config_test

import (
	"errors"
	"os"
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockFileSystem is a local mock implementing config.FileSystem for testing
type mockFileSystem struct {
	Files       map[string][]byte
	HomeDirErr  error
	ReadFileErr error
}

func (m *mockFileSystem) UserHomeDir() (string, error) {
	if m.HomeDirErr != nil {
		return "", m.HomeDirErr
	}
	return "/home/user", nil
}

func (m *mockFileSystem) ReadFile(path string) ([]byte, error) {
	if m.ReadFileErr != nil {
		return nil, m.ReadFileErr
	}
	if data, ok := m.Files[path]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

func (m *mockFileSystem) WriteFile(path string, data []byte, perm os.FileMode) error {
	if m.Files == nil {
		m.Files = make(map[string][]byte)
	}
	m.Files[path] = data
	return nil
}

func (m *mockFileSystem) MkdirAll(path string, perm os.FileMode) error {
	// For mock purposes, we assume directory creation always succeeds
	return nil
}

// SetOperationError sets an error for a specific operation
func (m *mockFileSystem) SetOperationError(operation string, err error) {
	if operation == "ReadFile" {
		m.ReadFileErr = err
	}
}

// createMockFS helper to reduce boilerplate
func createMockFS(files map[string][]byte) *mockFileSystem {
	return &mockFileSystem{
		Files: files,
	}
}

// --- HAPPY PATH TESTS ---

func TestLoad_NoConfigFile_ReturnsDefaults(t *testing.T) {
	// Config file doesn't exist - should return all defaults
	fs := createMockFS(nil)
	loader := config.NewLoaderWithFS(fs)

	cfg, err := loader.Load()

	require.NoError(t, err)
	assert.Equal(t, int64(20*1024*1024), cfg.Tools.MaxFileSize)
}

func TestLoad_FullOverride_AllValuesReplaced(t *testing.T) {
	// Config file overrides fields
	configJSON := `{
		"tools": {"max_file_size": 10485760, "default_shell_timeout": 1800}
	}`
	fs := createMockFS(map[string][]byte{
		"/home/user/.config/iav/config.json": []byte(configJSON),
	})
	loader := config.NewLoaderWithFS(fs)

	cfg, err := loader.Load()

	require.NoError(t, err)
	assert.Equal(t, int64(10485760), cfg.Tools.MaxFileSize)
	assert.Equal(t, 1800, cfg.Tools.DefaultShellTimeout)
}

func TestLoad_PartialOverride_MergesWithDefaults(t *testing.T) {
	// Config file only overrides default_shell_timeout - rest should be defaults
	configJSON := `{"tools": {"default_shell_timeout": 1200}}`
	fs := createMockFS(map[string][]byte{
		"/home/user/.config/iav/config.json": []byte(configJSON),
	})
	loader := config.NewLoaderWithFS(fs)

	cfg, err := loader.Load()

	require.NoError(t, err)
	assert.Equal(t, 1200, cfg.Tools.DefaultShellTimeout)        // Overridden
	assert.Equal(t, int64(20*1024*1024), cfg.Tools.MaxFileSize) // Default
}

func TestLoad_EmptyConfigFile_ReturnsDefaults(t *testing.T) {
	// Empty JSON object - should use all defaults
	fs := createMockFS(map[string][]byte{
		"/home/user/.config/iav/config.json": []byte(`{}`),
	})
	loader := config.NewLoaderWithFS(fs)

	cfg, err := loader.Load()

	require.NoError(t, err)
	assert.Equal(t, int64(20*1024*1024), cfg.Tools.MaxFileSize)
}

// --- UNHAPPY PATH TESTS ---

func TestLoad_MalformedJSON_ReturnsError(t *testing.T) {
	fs := createMockFS(map[string][]byte{
		"/home/user/.config/iav/config.json": []byte(`{invalid json`),
	})
	loader := config.NewLoaderWithFS(fs)

	cfg, err := loader.Load()

	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "invalid")
}

func TestLoad_PermissionDenied_ReturnsError(t *testing.T) {
	fs := createMockFS(nil)
	fs.SetOperationError("ReadFile", os.ErrPermission)

	loader := config.NewLoaderWithFS(fs)

	cfg, err := loader.Load()

	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.True(t, errors.Is(err, os.ErrPermission))
}

func TestLoad_HomeDirError_ReturnsDefaults(t *testing.T) {
	// Can't determine home dir - gracefully fall back to defaults
	fs := createMockFS(nil)
	fs.HomeDirErr = errors.New("homeless")

	loader := config.NewLoaderWithFS(fs)

	cfg, err := loader.Load()

	require.NoError(t, err)
	assert.Equal(t, int64(20*1024*1024), cfg.Tools.MaxFileSize) // Default
}

func TestLoad_WrongJSONType_ReturnsError(t *testing.T) {
	// JSON is valid but wrong type (array instead of object)
	fs := createMockFS(map[string][]byte{
		"/home/user/.config/iav/config.json": []byte(`["not", "an", "object"]`),
	})
	loader := config.NewLoaderWithFS(fs)

	cfg, err := loader.Load()

	assert.Error(t, err)
	assert.Nil(t, cfg)
}

// --- EDGE CASE TESTS ---

func TestLoad_NegativeValues_Rejected(t *testing.T) {
	// Negative values should be rejected by validation
	configJSON := `{"tools": {"default_shell_timeout": -1}}`
	fs := createMockFS(map[string][]byte{
		"/home/user/.config/iav/config.json": []byte(configJSON),
	})
	loader := config.NewLoaderWithFS(fs)

	cfg, err := loader.Load()

	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestLoad_UnknownFields_Ignored(t *testing.T) {
	// Unknown fields in JSON should be silently ignored
	configJSON := `{"tools": {"max_file_size": 1024}, "unknown_field": "ignored"}`
	fs := createMockFS(map[string][]byte{
		"/home/user/.config/iav/config.json": []byte(configJSON),
	})
	loader := config.NewLoaderWithFS(fs)

	cfg, err := loader.Load()

	require.NoError(t, err)
	assert.Equal(t, int64(1024), cfg.Tools.MaxFileSize)
}

func TestLoad_NoConfigFile_DoesNotCreateFile(t *testing.T) {
	fs := createMockFS(nil)
	loader := config.NewLoaderWithFS(fs)

	_, err := loader.Load()
	require.NoError(t, err)

	// Verify no file was created
	assert.Empty(t, fs.Files)
}

func TestLoadState_NoFile_ReturnsDefaults(t *testing.T) {
	fs := createMockFS(nil)
	loader := config.NewLoaderWithFS(fs)

	state, err := loader.LoadState()
	require.NoError(t, err)
	assert.NotNil(t, state)
	assert.Empty(t, state.CurrentSessionID)
}

func TestSaveState_PersistsToFile(t *testing.T) {
	fs := createMockFS(nil)
	loader := config.NewLoaderWithFS(fs)

	state := &config.State{CurrentSessionID: "test-session"}
	err := loader.SaveState(state)
	require.NoError(t, err)

	// Verify file was created in the correct location
	data, ok := fs.Files["/home/user/.config/iav/state.json"]
	assert.True(t, ok)
	assert.Contains(t, string(data), "test-session")
}

func TestLoadState_ReadsFromFile(t *testing.T) {
	stateJSON := `{"current_session_id": "persisted-session"}`
	fs := createMockFS(map[string][]byte{
		"/home/user/.config/iav/state.json": []byte(stateJSON),
	})
	loader := config.NewLoaderWithFS(fs)

	state, err := loader.LoadState()
	require.NoError(t, err)
	assert.Equal(t, "persisted-session", state.CurrentSessionID)
}
