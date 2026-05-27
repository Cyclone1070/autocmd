package checksum

import (
	"maps"
	"sync"
	"testing"
)

func TestChecksumManager(t *testing.T) {
	manager := NewManager(nil, "")

	path := "/test/path.txt"
	checksum := "abc123"

	// Test Get on empty cache
	_, ok := manager.Get(path)
	if ok {
		t.Error("cache should be empty")
	}

	// Test Update
	manager.Update(path, checksum)

	// Test Get after update
	retrievedChecksum, ok := manager.Get(path)
	if !ok {
		t.Error("cache should contain the entry")
	}
	if retrievedChecksum != checksum {
		t.Errorf("expected checksum %s, got %s", checksum, retrievedChecksum)
	}

	// Test concurrent access
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(_ int) {
			defer wg.Done()
			manager.Update(path, checksum)
			manager.Get(path)
		}(i)
	}
	wg.Wait()

	// Verify final state
	finalChecksum, ok := manager.Get(path)
	if !ok {
		t.Error("cache entry should still exist after concurrent access")
	}
	if finalChecksum != checksum {
		t.Errorf("checksum should remain %s after concurrent access", checksum)
	}
}

func TestCompute(t *testing.T) {
	manager := NewManager(nil, "")

	t.Run("EmptyData", func(t *testing.T) {
		data := []byte("")
		hash := manager.Compute(data)
		// echo -n "" | shasum -a 256
		expected := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
		if hash != expected {
			t.Errorf("got %s, want %s", hash, expected)
		}
	})

	t.Run("KnownHash", func(t *testing.T) {
		data := []byte("hello")
		hash := manager.Compute(data)
		// echo -n "hello" | shasum -a 256
		expected := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
		if hash != expected {
			t.Errorf("got %s, want %s", hash, expected)
		}
	})
}

type mockChecksumStore struct {
	data map[string]map[string]string
}

func (m *mockChecksumStore) LoadChecksums(sessionID string) (map[string]string, error) {
	if m.data == nil {
		return make(map[string]string), nil
	}
	checksums, ok := m.data[sessionID]
	if !ok {
		return make(map[string]string), nil
	}
	copied := make(map[string]string)
	maps.Copy(copied, checksums)
	return copied, nil
}

func (m *mockChecksumStore) SaveChecksums(sessionID string, checksums map[string]string) error {
	if m.data == nil {
		m.data = make(map[string]map[string]string)
	}
	copied := make(map[string]string)
	maps.Copy(copied, checksums)
	m.data[sessionID] = copied
	return nil
}

func TestPersistentChecksumManager(t *testing.T) {
	store := &mockChecksumStore{}

	manager1 := NewManager(store, "session-123")

	// Update (should save dynamically to store)
	manager1.Update("/file1.txt", "hash1")

	// Create manager2 sharing the same store and state
	manager2 := NewManager(store, "session-123")

	// Get (should load dynamically from store)
	h1, ok := manager2.Get("/file1.txt")
	if !ok || h1 != "hash1" {
		t.Errorf("expected hash1, got %q (ok=%t)", h1, ok)
	}
}

