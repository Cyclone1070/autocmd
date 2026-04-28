package hash

import (
	"sync"
	"testing"
)

func TestChecksumManager(t *testing.T) {
	manager := NewChecksumManager()
	manager.Clear()

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

func TestChecksumManagerClear(t *testing.T) {
	manager := NewChecksumManager()
	manager.Clear()

	// Add some entries
	manager.Update("/file1.txt", "hash1")
	manager.Update("/file2.txt", "hash2")

	// Clear
	manager.Clear()

	// Verify entries are gone
	_, ok := manager.Get("/file1.txt")
	if ok {
		t.Error("cache should be empty after Clear")
	}
}

func TestCompute(t *testing.T) {
	manager := NewChecksumManager()

	t.Run("EmptyData", func(t *testing.T) {
		hash := manager.Compute([]byte{})
		expected := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" // known empty sha256
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
