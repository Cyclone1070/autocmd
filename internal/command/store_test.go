package command

import (
	"os"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type mockFileSystem struct {
	files     map[string][]byte
	writeErr  error
	readErr   error
	ensureErr error
	removeErr error
}

func newMockFS() *mockFileSystem {
	return &mockFileSystem{files: make(map[string][]byte)}
}

func (m *mockFileSystem) WriteFileAtomic(path string, data []byte, _ os.FileMode) error {
	if m.writeErr != nil {
		return m.writeErr
	}
	m.files[path] = data
	return nil
}

func (m *mockFileSystem) ReadFile(path string) ([]byte, error) {
	if m.readErr != nil {
		return nil, m.readErr
	}
	data, ok := m.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}

func (m *mockFileSystem) EnsureDirs(_ string) error {
	if m.ensureErr != nil {
		return m.ensureErr
	}
	return nil
}

func (m *mockFileSystem) Remove(path string) error {
	if m.removeErr != nil {
		return m.removeErr
	}
	delete(m.files, path)
	return nil
}

func (m *mockFileSystem) RemoveAll(path string) error {
	return m.Remove(path)
}

func newTestStore() *Store {
	return NewStore(newMockFS(), "/test/path/commands.json")
}

func TestStore_Save_NewCommand(t *testing.T) {
	store := newTestStore()

	err := store.Save("git imp", "git status --porcelain", "Show git status without test files")
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	cmd, ok := store.Get("git imp")
	if !ok {
		t.Fatal("Get() should return true for saved command")
	}
	if cmd.Name != "git imp" {
		t.Errorf("Name = %q, want %q", cmd.Name, "git imp")
	}
	if cmd.Command != "git status --porcelain" {
		t.Errorf("Command = %q, want %q", cmd.Command, "git status --porcelain")
	}
	if cmd.Description != "Show git status without test files" {
		t.Errorf("Description = %q, want %q", cmd.Description, "Show git status without test files")
	}
	if cmd.CreatedAt == 0 {
		t.Error("CreatedAt should be set")
	}
	if cmd.UpdatedAt == 0 {
		t.Error("UpdatedAt should be set")
	}
	if cmd.CreatedAt != cmd.UpdatedAt {
		t.Errorf("CreatedAt (%d) should equal UpdatedAt (%d) for new command", cmd.CreatedAt, cmd.UpdatedAt)
	}
}

func TestStore_Save_Overwrite(t *testing.T) {
	store := newTestStore()

	err := store.Save("git imp", "git status --porcelain", "Original desc")
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Small delay so timestamps differ
	time.Sleep(time.Millisecond)

	err = store.Save("git imp", "git status -s", "Updated description")
	if err != nil {
		t.Fatalf("Save() overwrite failed: %v", err)
	}

	cmd, ok := store.Get("git imp")
	if !ok {
		t.Fatal("Get() should return true after overwrite")
	}
	if cmd.Command != "git status -s" {
		t.Errorf("Command = %q, want %q", cmd.Command, "git status -s")
	}
	if cmd.Description != "Updated description" {
		t.Errorf("Description = %q, want %q", cmd.Description, "Updated description")
	}
	// CreatedAt should be preserved from first save
	if cmd.CreatedAt == 0 {
		t.Error("CreatedAt should still be set")
	}
	if cmd.UpdatedAt <= cmd.CreatedAt {
		t.Errorf("UpdatedAt (%d) should be > CreatedAt (%d) after overwrite", cmd.UpdatedAt, cmd.CreatedAt)
	}
}

func TestStore_Get_NonExistent(t *testing.T) {
	store := newTestStore()

	_, ok := store.Get("nonexistent")
	if ok {
		t.Error("Get() should return false for non-existent command")
	}
}

func TestStore_List_Empty(t *testing.T) {
	store := newTestStore()

	cmds := store.List()
	if cmds == nil {
		t.Error("List() should return empty slice, not nil")
	}
	if len(cmds) != 0 {
		t.Errorf("List() = %d items, want 0", len(cmds))
	}
}

func TestStore_List_Multiple(t *testing.T) {
	store := newTestStore()

	require.NoError(t, store.Save("first", "echo first", ""))
	require.NoError(t, store.Save("second", "echo second", "Second command"))
	require.NoError(t, store.Save("third", "echo third", ""))

	cmds := store.List()
	if len(cmds) != 3 {
		t.Fatalf("List() = %d items, want 3", len(cmds))
	}

	// Should be sorted by name
	sort.Slice(cmds, func(i, j int) bool {
		return cmds[i].Name < cmds[j].Name
	})

	if cmds[0].Name != "first" {
		t.Errorf("First = %q, want %q", cmds[0].Name, "first")
	}
	if cmds[1].Name != "second" {
		t.Errorf("Second = %q, want %q", cmds[1].Name, "second")
	}
	if cmds[2].Name != "third" {
		t.Errorf("Third = %q, want %q", cmds[2].Name, "third")
	}
}

func TestStore_Delete_Success(t *testing.T) {
	store := newTestStore()

	require.NoError(t, store.Save("to-delete", "echo delete me", ""))
	ok := store.Delete("to-delete")
	if !ok {
		t.Error("Delete() should return true when command existed")
	}
	_, ok = store.Get("to-delete")
	if ok {
		t.Error("Get() should return false after Delete()")
	}
}

func TestStore_Delete_NonExistent(t *testing.T) {
	store := newTestStore()

	ok := store.Delete("nonexistent")
	if ok {
		t.Error("Delete() should return false for non-existent command")
	}
}

func TestStore_Persistence(t *testing.T) {
	fs := newMockFS()
	store := NewStore(fs, "/test/path/commands.json")

	// Save a command
	err := store.Save("hello", "echo hello world", "Greeting")
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Verify data was written to the file
	data, ok := fs.files["/test/path/commands.json"]
	if !ok {
		t.Fatal("commands.json was not written to filesystem")
	}
	if len(data) == 0 {
		t.Fatal("commands.json is empty")
	}

	// Create a new store reading from same filesystem
	store2 := NewStore(fs, "/test/path/commands.json")
	cmd, ok := store2.Get("hello")
	if !ok {
		t.Fatal("Get() after reload should return true")
	}
	if cmd.Command != "echo hello world" {
		t.Errorf("Command = %q, want %q", cmd.Command, "echo hello world")
	}
}

func TestStore_Persistence_LoadNonExistent(t *testing.T) {
	fs := newMockFS()
	store := NewStore(fs, "/test/path/commands.json")

	// Should not error, should return empty
	cmds := store.List()
	if len(cmds) != 0 {
		t.Errorf("List() = %d items, want 0", len(cmds))
	}
}

func TestStore_Persistence_CorruptedFile(t *testing.T) {
	fs := newMockFS()
	fs.files["/test/path/commands.json"] = []byte("invalid json{{")

	store := NewStore(fs, "/test/path/commands.json")

	// Should return empty data, not crash
	_, ok := store.Get("anything")
	if ok {
		t.Error("Get() should return false for corrupted file")
	}
}

func TestStore_IsEmptyOrWhitespaceName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty", "", true},
		{"whitespace", "   ", true},
		{"valid", "git imp", false},
		{"single char", "x", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isEmptyOrWhitespace(tc.input)
			if got != tc.want {
				t.Errorf("isEmptyOrWhitespace(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestStore_DeleteAll(t *testing.T) {
	store := newTestStore()

	require.NoError(t, store.Save("a", "echo a", ""))
	require.NoError(t, store.Save("b", "echo b", ""))

	store.DeleteAll()

	cmds := store.List()
	if len(cmds) != 0 {
		t.Errorf("List() after DeleteAll = %d items, want 0", len(cmds))
	}
}
