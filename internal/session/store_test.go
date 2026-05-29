package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
)

const (
	testSessionID = "test-session"
	session1ID    = "session-1"
)

// mockDirEntry implements os.DirEntry for testing.
type mockDirEntry struct {
	name  string
	isDir bool
}

func (m *mockDirEntry) Name() string               { return m.name }
func (m *mockDirEntry) IsDir() bool                { return m.isDir }
func (m *mockDirEntry) Type() os.FileMode          { return 0 }
func (m *mockDirEntry) Info() (os.FileInfo, error) { return nil, errors.New("not implemented") }

// mockFileSystem implements fileSystem interface for testing.
type mockFileSystem struct {
	readErr         error
	writeErr        error
	removeErr       error
	dirErr          error
	ensureErr       error
	files           map[string][]byte
	dirs            map[string][]os.DirEntry
	lastRemovedPath string
	writeCallCount  int
	failWriteAfter  int
}

func newMockFileSystem() *mockFileSystem {
	return &mockFileSystem{
		files: make(map[string][]byte),
		dirs:  make(map[string][]os.DirEntry),
	}
}

func (m *mockFileSystem) WriteFileAtomic(path string, content []byte, _ os.FileMode) error {
	m.writeCallCount++
	if m.failWriteAfter > 0 && m.writeCallCount > m.failWriteAfter {
		return errors.New("write failed")
	}
	if m.writeErr != nil {
		return m.writeErr
	}
	m.files[path] = content
	return nil
}

func (m *mockFileSystem) EnsureDirs(_ string) error {
	if m.ensureErr != nil {
		return m.ensureErr
	}
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

func (m *mockFileSystem) ListDir(path string) ([]os.DirEntry, error) {
	if m.dirErr != nil {
		return nil, m.dirErr
	}
	entries, ok := m.dirs[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return entries, nil
}

func (m *mockFileSystem) Remove(path string) error {
	if m.removeErr != nil {
		return m.removeErr
	}
	m.lastRemovedPath = path
	delete(m.files, path)
	return nil
}

func (m *mockFileSystem) RemoveAll(path string) error {
	if m.removeErr != nil {
		return m.removeErr
	}
	m.lastRemovedPath = path
	delete(m.files, path)
	return nil
}

func newTestStore() (*Store, *mockFileSystem) {
	fs := newMockFileSystem()
	return NewStore(fs, "/test/sessions"), fs
}



func TestCreate_Success(t *testing.T) {
	store, fs := newTestStore()

	sess, err := store.Create()
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	if sess.ID == "" {
		t.Error("Session ID should not be empty")
	}
	if sess.Name != "" {
		t.Errorf("Session Name should be empty, got %q", sess.Name)
	}
	if len(sess.Messages) != 0 {
		t.Errorf("Session Messages should be empty, got %d", len(sess.Messages))
	}

	// Verify files were written
	infoPath := filepath.Join(store.storageDir, sess.ID, "metadata.json")
	if _, ok := fs.files[infoPath]; !ok {
		t.Error("Info file should have been written to metadata.json in session subfolder")
	}
	messagesPath := filepath.Join(store.storageDir, sess.ID, "messages.json")
	if _, ok := fs.files[messagesPath]; !ok {
		t.Error("Messages file should have been written to messages.json in session subfolder")
	}
}

func TestCreate_EnsureDirsFails(t *testing.T) {
	store, fs := newTestStore()
	fs.ensureErr = errors.New("mkdir failed")

	_, err := store.Create()
	if err == nil {
		t.Fatal("Create() should have failed")
	}
	if err.Error() != "create storage dir: mkdir failed" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestCreate_SaveFails(t *testing.T) {
	store, fs := newTestStore()
	fs.writeErr = errors.New("write failed")

	_, err := store.Create()
	if err == nil {
		t.Fatal("Create() should have failed")
	}
	if err.Error() != "write session info: write failed" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestGetSession_SplitSuccess(t *testing.T) {
	store, fs := newTestStore()

	sessID := testSessionID
	now := time.Now()

	infoDTO := sessionInfoDTO{
		ID:           sessID,
		Name:         "Test Session",
		MessageCount: 2,
		Created:      now.UnixMilli(),
		Updated:      now.UnixMilli(),
	}
	infoData, _ := json.MarshalIndent(infoDTO, "", "  ")

	// Messages as direct JSON array
	messages := []*schema.Message{
		{Role: schema.User, Content: "Hello"},
		{Role: schema.Assistant, Content: "Hi there"},
	}
	messagesData, _ := json.MarshalIndent(messages, "", "  ")

	// Displays as separate displays.json
	displays := map[string]domain.ToolDisplay{
		"call-1": domain.NewStringDisplay("desc", "content"),
	}
	displaysData, _ := json.MarshalIndent(displays, "", "  ")

	infoPath := filepath.Join(store.storageDir, sessID, "metadata.json")
	messagesPath := filepath.Join(store.storageDir, sessID, "messages.json")
	displaysPath := filepath.Join(store.storageDir, sessID, "displays.json")

	fs.files[infoPath] = infoData
	fs.files[messagesPath] = messagesData
	fs.files[displaysPath] = displaysData

	sess, err := store.GetSession(sessID)
	if err != nil {
		t.Fatalf("GetSession() failed: %v", err)
	}

	if sess.ID != sessID {
		t.Errorf("Session ID mismatch: got %q, want %q", sess.ID, sessID)
	}
	if sess.Name != "Test Session" {
		t.Errorf("Session Name mismatch: got %q, want %q", sess.Name, "Test Session")
	}
	if len(sess.Messages) != 2 {
		t.Errorf("Session Messages count mismatch: got %d, want 2", len(sess.Messages))
	}
	if len(sess.ToolDisplays) != 1 {
		t.Fatalf("expected 1 tool display, got %d", len(sess.ToolDisplays))
	}
	d, ok := sess.ToolDisplays["call-1"].(domain.StringDisplay)
	if !ok || d.Content != "content" {
		t.Errorf("Session ToolDisplays mismatch: got %v", sess.ToolDisplays)
	}
}


func TestGetSession_NotFound(t *testing.T) {
	store, _ := newTestStore()

	_, err := store.GetSession("nonexistent")
	if err == nil {
		t.Fatal("GetSession() should have failed")
	}
	if err.Error() != "read session info: file does not exist" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestGetSession_CorruptedInfoJSON(t *testing.T) {
	store, fs := newTestStore()

	sessID := testSessionID
	infoPath := filepath.Join(store.storageDir, sessID, "metadata.json")
	fs.files[infoPath] = []byte("invalid json{")

	_, err := store.GetSession(sessID)
	if err == nil {
		t.Fatal("GetSession() should have failed")
	}
	if err.Error() != "unmarshal session info: invalid character 'i' looking for beginning of value" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestGetSession_CorruptedMessagesJSON(t *testing.T) {
	store, fs := newTestStore()

	sessID := testSessionID
	now := time.Now()

	infoDTO := sessionInfoDTO{
		ID:           sessID,
		Name:         "Test",
		MessageCount: 0,
		Created:      now.UnixMilli(),
		Updated:      now.UnixMilli(),
	}
	infoData, _ := json.MarshalIndent(infoDTO, "", "  ")

	infoPath := filepath.Join(store.storageDir, sessID, "metadata.json")
	messagesPath := filepath.Join(store.storageDir, sessID, "messages.json")

	fs.files[infoPath] = infoData
	fs.files[messagesPath] = []byte("invalid json{")

	_, err := store.GetSession(sessID)
	if err == nil {
		t.Fatal("GetSession() should have failed")
	}
	if err.Error() != "unmarshal session messages: invalid character 'i' looking for beginning of value" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestGetSession_MessagesFileMissing(t *testing.T) {
	store, fs := newTestStore()

	sessID := testSessionID
	now := time.Now()

	infoDTO := sessionInfoDTO{
		ID:           sessID,
		Name:         "Test",
		MessageCount: 0,
		Created:      now.UnixMilli(),
		Updated:      now.UnixMilli(),
	}
	infoData, _ := json.MarshalIndent(infoDTO, "", "  ")

	infoPath := filepath.Join(store.storageDir, sessID, "metadata.json")
	fs.files[infoPath] = infoData

	sess, err := store.GetSession(sessID)
	if err != nil {
		t.Fatalf("GetSession() failed: %v", err)
	}

	if len(sess.Messages) != 0 {
		t.Errorf("Session Messages should be empty when messages file is missing, got %d", len(sess.Messages))
	}
}

func TestSaveSession_Success(t *testing.T) {
	store, fs := newTestStore()

	sess := &domain.Session{
		SessionMetadata: domain.SessionMetadata{
			ID:      testSessionID,
			Name:    "Test Session",
			Created: time.Now(),
			Updated: time.Now(),
		},
		SessionMessages: domain.SessionMessages{
			Messages: []*schema.Message{
				{Role: schema.User, Content: "Hello"},
			},
		},
		SessionDisplays: domain.SessionDisplays{
			ToolDisplays: map[string]domain.ToolDisplay{
				"call-1": domain.NewStringDisplay("desc", "content"),
			},
		},
	}

	err := store.SaveSession(sess)
	if err != nil {
		t.Fatalf("SaveSession() failed: %v", err)
	}

	infoPath := filepath.Join(store.storageDir, sess.ID, "metadata.json")
	messagesPath := filepath.Join(store.storageDir, sess.ID, "messages.json")
	displaysPath := filepath.Join(store.storageDir, sess.ID, "displays.json")

	if _, ok := fs.files[infoPath]; !ok {
		t.Error("Info file should have been written")
	}
	if _, ok := fs.files[messagesPath]; !ok {
		t.Error("Messages file should have been written")
	}
	if _, ok := fs.files[displaysPath]; !ok {
		t.Error("Displays file should have been written")
	}

	// Verify displays content
	var savedDisplays domain.SessionDisplays
	if err := json.Unmarshal(fs.files[displaysPath], &savedDisplays); err != nil {
		t.Fatalf("failed to unmarshal displays: %v", err)
	}
	if len(savedDisplays.ToolDisplays) != 1 {
		t.Fatalf("saved displays count mismatch: %d", len(savedDisplays.ToolDisplays))
	}
	d, ok := savedDisplays.ToolDisplays["call-1"].(domain.StringDisplay)
	if !ok || d.Content != "content" {
		t.Errorf("saved displays mismatch: %v", savedDisplays.ToolDisplays)
	}

	// Verify messages content (should be direct array)
	var savedMessages []*schema.Message
	if err := json.Unmarshal(fs.files[messagesPath], &savedMessages); err != nil {
		t.Fatalf("failed to unmarshal messages: %v", err)
	}
	if len(savedMessages) != 1 || savedMessages[0].Content != "Hello" {
		t.Errorf("saved messages mismatch: %v", savedMessages)
	}

	// Verify Updated timestamp was set
	if sess.Updated.IsZero() {
		t.Error("Session Updated timestamp should have been set")
	}
}

func TestSaveSession_InfoWriteFails(t *testing.T) {
	store, fs := newTestStore()
	fs.writeErr = errors.New("write failed")

	sess := &domain.Session{
		SessionMetadata: domain.SessionMetadata{
			ID:     testSessionID,
			Name:   "Test",
		},
		SessionMessages: domain.SessionMessages{Messages: []*schema.Message{}},
		SessionDisplays: domain.SessionDisplays{ToolDisplays: map[string]domain.ToolDisplay{}},
	}

	err := store.SaveSession(sess)
	if err == nil {
		t.Fatal("SaveSession() should have failed")
	}
	if err.Error() != "write session info: write failed" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestSaveSession_MessagesWriteFails(t *testing.T) {
	store, fs := newTestStore()

	sess := &domain.Session{
		SessionMetadata: domain.SessionMetadata{
			ID:     testSessionID,
			Name:   "Test",
		},
		SessionMessages: domain.SessionMessages{Messages: []*schema.Message{}},
		SessionDisplays: domain.SessionDisplays{ToolDisplays: map[string]domain.ToolDisplay{}},
	}

	// First write succeeds, second fails
	fs.failWriteAfter = 1

	err := store.SaveSession(sess)
	if err == nil {
		t.Fatal("SaveSession() should have failed")
	}
	if err.Error() != "write session messages: write failed" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestList_Success(t *testing.T) {
	store, fs := newTestStore()

	now := time.Now()
	sess1ID := session1ID
	sess2ID := "session-2"

	// Create two valid sessions
	info1DTO := sessionInfoDTO{
		ID:           sess1ID,
		Name:         "Session 1",
		MessageCount: 5,
		Created:      now.Add(-2 * time.Hour).UnixMilli(),
		Updated:      now.Add(-1 * time.Hour).UnixMilli(),
	}
	info1Data, _ := json.MarshalIndent(info1DTO, "", "  ")

	info2DTO := sessionInfoDTO{
		ID:           sess2ID,
		Name:         "Session 2",
		MessageCount: 3,
		Created:      now.Add(-1 * time.Hour).UnixMilli(),
		Updated:      now.UnixMilli(),
	}
	info2Data, _ := json.MarshalIndent(info2DTO, "", "  ")

	info1Path := filepath.Join(store.storageDir, sess1ID, "metadata.json")
	info2Path := filepath.Join(store.storageDir, sess2ID, "metadata.json")

	fs.files[info1Path] = info1Data
	fs.files[info2Path] = info2Data

	// Create directory entries (List now iterates over session directories)
	fs.dirs[store.storageDir] = []os.DirEntry{
		&mockDirEntry{name: sess1ID, isDir: true},
		&mockDirEntry{name: sess2ID, isDir: true},
	}

	summaries, err := store.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	if len(summaries) != 2 {
		t.Fatalf("Expected 2 summaries, got %d", len(summaries))
	}

	// Verify sorted by updated time (newest first)
	if summaries[0].ID != sess2ID {
		t.Errorf("First summary should be session-2 (newest), got %q", summaries[0].ID)
	}
	if summaries[1].ID != sess1ID {
		t.Errorf("Second summary should be session-1 (older), got %q", summaries[1].ID)
	}
}

func TestList_EmptyDir(t *testing.T) {
	store, fs := newTestStore()

	fs.dirs[store.storageDir] = []os.DirEntry{}

	summaries, err := store.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	if len(summaries) != 0 {
		t.Errorf("Expected empty summaries, got %d", len(summaries))
	}
}

func TestList_DirNotExists(t *testing.T) {
	store, _ := newTestStore()

	summaries, err := store.List()
	if err != nil {
		t.Fatalf("List() should return empty slice when dir doesn't exist, got error: %v", err)
	}

	if len(summaries) != 0 {
		t.Errorf("Expected empty summaries, got %d", len(summaries))
	}
}

func TestList_SkipsMessagesFiles(t *testing.T) {
	store, fs := newTestStore()

	sessID := session1ID
	now := time.Now()

	infoDTO := sessionInfoDTO{
		ID:           sessID,
		Name:         "Session 1",
		MessageCount: 2,
		Created:      now.UnixMilli(),
		Updated:      now.UnixMilli(),
	}
	infoData, _ := json.MarshalIndent(infoDTO, "", "  ")

	infoPath := filepath.Join(store.storageDir, sessID, "metadata.json")
	fs.files[infoPath] = infoData

	// Directory contains session directories, not .json files
	fs.dirs[store.storageDir] = []os.DirEntry{
		&mockDirEntry{name: sessID, isDir: true},
		&mockDirEntry{name: "some-other-dir", isDir: true}, // metadata.json missing in some-other-dir, should skip
	}

	summaries, err := store.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	if len(summaries) != 1 {
		t.Errorf("Expected 1 summary (skipping directory without metadata), got %d", len(summaries))
	}
}

func TestList_SkipsCorruptedFiles(t *testing.T) {
	store, fs := newTestStore()

	sess1ID := session1ID
	sess2ID := "session-2"
	now := time.Now()

	// Valid session
	info1DTO := sessionInfoDTO{
		ID:           sess1ID,
		Name:         "Session 1",
		MessageCount: 2,
		Created:      now.UnixMilli(),
		Updated:      now.UnixMilli(),
	}
	info1Data, _ := json.MarshalIndent(info1DTO, "", "  ")

	info1Path := filepath.Join(store.storageDir, sess1ID, "metadata.json")
	fs.files[info1Path] = info1Data

	// Corrupted session
	info2Path := filepath.Join(store.storageDir, sess2ID, "metadata.json")
	fs.files[info2Path] = []byte("invalid json{")

	fs.dirs[store.storageDir] = []os.DirEntry{
		&mockDirEntry{name: sess1ID, isDir: true},
		&mockDirEntry{name: sess2ID, isDir: true},
	}

	summaries, err := store.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	if len(summaries) != 1 {
		t.Errorf("Expected 1 summary (skipping corrupted), got %d", len(summaries))
	}
	if summaries[0].ID != sess1ID {
		t.Errorf("Expected session-1, got %q", summaries[0].ID)
	}
}

func TestList_SortsByUpdatedDesc(t *testing.T) {
	store, fs := newTestStore()

	now := time.Now()
	sessions := []struct {
		updated time.Time
		id      string
	}{
		{id: "oldest", updated: now.Add(-3 * time.Hour)},
		{id: "middle", updated: now.Add(-1 * time.Hour)},
		{id: "newest", updated: now},
	}

	entries := make([]os.DirEntry, 0, len(sessions))
	for _, s := range sessions {
		infoDTO := sessionInfoDTO{
			ID:           s.id,
			Name:         s.id,
			MessageCount: 1,
			Created:      s.updated.UnixMilli(),
			Updated:      s.updated.UnixMilli(),
		}
		infoData, _ := json.MarshalIndent(infoDTO, "", "  ")
		infoPath := filepath.Join(store.storageDir, s.id, "metadata.json")
		fs.files[infoPath] = infoData
		entries = append(entries, &mockDirEntry{name: s.id, isDir: true})
	}

	fs.dirs[store.storageDir] = entries

	summaries, err := store.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	if len(summaries) != 3 {
		t.Fatalf("Expected 3 summaries, got %d", len(summaries))
	}

	// Verify sorted descending by Updated
	if summaries[0].ID != "newest" {
		t.Errorf("First should be newest, got %q", summaries[0].ID)
	}
	if summaries[1].ID != "middle" {
		t.Errorf("Second should be middle, got %q", summaries[1].ID)
	}
	if summaries[2].ID != "oldest" {
		t.Errorf("Third should be oldest, got %q", summaries[2].ID)
	}

	// Verify sort order
	if !summaries[0].Updated.After(summaries[1].Updated) {
		t.Error("First summary should be newer than second")
	}
	if !summaries[1].Updated.After(summaries[2].Updated) {
		t.Error("Second summary should be newer than third")
	}
}

func TestDelete_Success(t *testing.T) {
	store, fs := newTestStore()

	sessID := testSessionID
	sessDir := filepath.Join(store.storageDir, sessID)
	infoPath := filepath.Join(sessDir, "metadata.json")
	messagesPath := filepath.Join(sessDir, "messages.json")

	fs.files[infoPath] = []byte("info")
	fs.files[messagesPath] = []byte("messages")

	err := store.Delete(sessID)
	if err != nil {
		t.Fatalf("Delete() failed: %v", err)
	}

	// Delete in subfolder mode means we Remove the whole directory
	if fs.lastRemovedPath != sessDir {
		t.Errorf("Expected directory %q to be removed, got %q", sessDir, fs.lastRemovedPath)
	}
}

func TestDelete_FilesNotExist(t *testing.T) {
	store, _ := newTestStore()

	// Delete non-existent session should succeed
	err := store.Delete("nonexistent")
	if err != nil {
		t.Fatalf("Delete() should succeed even if files don't exist, got error: %v", err)
	}
}

func TestDelete_RemoveFails(t *testing.T) {
	store, fs := newTestStore()

	sessID := testSessionID
	sessDir := filepath.Join(store.storageDir, sessID)
	fs.files[filepath.Join(sessDir, "metadata.json")] = []byte("info")

	fs.removeErr = errors.New("remove failed")

	err := store.Delete(sessID)
	if err == nil {
		t.Fatal("Delete() should have failed")
	}
	if err.Error() != "remove failed" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestCreateSaveGetSessionRoundtrip(t *testing.T) {
	store, _ := newTestStore()

	// Create
	sess, err := store.Create()
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	// Modify
	sess.Name = "Roundtrip Test"
	sess.Messages = []*schema.Message{
		{Role: schema.User, Content: "Test message"},
	}

	// Save
	err = store.SaveSession(sess)
	if err != nil {
		t.Fatalf("SaveSession() failed: %v", err)
	}

	// Get
	loaded, err := store.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("GetSession() failed: %v", err)
	}

	// Verify
	if loaded.ID != sess.ID {
		t.Errorf("ID mismatch: got %q, want %q", loaded.ID, sess.ID)
	}
	if loaded.Name != sess.Name {
		t.Errorf("Name mismatch: got %q, want %q", loaded.Name, sess.Name)
	}
	if len(loaded.Messages) != len(sess.Messages) {
		t.Errorf("Messages count mismatch: got %d, want %d", len(loaded.Messages), len(sess.Messages))
	}
	if len(loaded.Messages) > 0 {
		content := loaded.Messages[0].Content
		sessContent := sess.Messages[0].Content
		if content != sessContent {
			t.Errorf("Message content mismatch: got %q, want %q", content, sessContent)
		}
	}
}

func TestList_SkipsSubdirectories(t *testing.T) {
	store, fs := newTestStore()

	sessID := session1ID
	now := time.Now()

	infoDTO := sessionInfoDTO{
		ID:           sessID,
		Name:         "Session 1",
		MessageCount: 1,
		Created:      now.UnixMilli(),
		Updated:      now.UnixMilli(),
	}
	infoData, _ := json.MarshalIndent(infoDTO, "", "  ")

	infoPath := filepath.Join(store.storageDir, sessID, "metadata.json")
	fs.files[infoPath] = infoData

	// Directory contains a file and a subdirectory
	fs.dirs[store.storageDir] = []os.DirEntry{
		&mockDirEntry{name: sessID, isDir: true},
		&mockDirEntry{name: "subdir", isDir: true},
	}

	summaries, err := store.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	if len(summaries) != 1 {
		t.Errorf("Expected 1 summary (skipping subdirectory), got %d", len(summaries))
	}
}

func TestList_ReadDirError(t *testing.T) {
	store, fs := newTestStore()
	fs.dirErr = errors.New("readdir failed")

	_, err := store.List()
	if err == nil {
		t.Fatal("List() should have failed")
	}
	if err.Error() != "read storage dir: readdir failed" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestList_SkipsNonJSONFiles(t *testing.T) {
	store, fs := newTestStore()

	sessID := session1ID
	now := time.Now()

	infoDTO := sessionInfoDTO{
		ID:           sessID,
		Name:         "Session 1",
		MessageCount: 1,
		Created:      now.UnixMilli(),
		Updated:      now.UnixMilli(),
	}
	infoData, _ := json.MarshalIndent(infoDTO, "", "  ")

	infoPath := filepath.Join(store.storageDir, sessID, "metadata.json")
	fs.files[infoPath] = infoData

	// Directory contains session subdirectories. Skips non-directory files like .txt.
	fs.dirs[store.storageDir] = []os.DirEntry{
		&mockDirEntry{name: sessID, isDir: true},
		&mockDirEntry{name: "file.txt", isDir: false}, // Skips files
	}

	summaries, err := store.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	if len(summaries) != 1 {
		t.Errorf("Expected 1 summary (skipping .txt file), got %d", len(summaries))
	}
}

func TestList_SkipsDirectoriesWithoutMetadata(t *testing.T) {
	store, fs := newTestStore()

	sessID := session1ID
	now := time.Now()

	infoDTO := sessionInfoDTO{
		ID:           sessID,
		Name:         "Session 1",
		MessageCount: 1,
		Created:      now.UnixMilli(),
		Updated:      now.UnixMilli(),
	}
	infoData, _ := json.MarshalIndent(infoDTO, "", "  ")

	infoPath := filepath.Join(store.storageDir, sessID, "metadata.json")
	fs.files[infoPath] = infoData

	// Directory contains one valid session and one directory without metadata.json
	fs.dirs[store.storageDir] = []os.DirEntry{
		&mockDirEntry{name: sessID, isDir: true},
		&mockDirEntry{name: "empty-dir", isDir: true},
	}

	summaries, err := store.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	if len(summaries) != 1 {
		t.Errorf("Expected 1 summary (skipping directory without metadata.json), got %d", len(summaries))
	}
}

func TestFindBlank(t *testing.T) {
	store, fs := newTestStore()

	// 1. Initially no blanks
	blank, err := store.FindBlank()
	if err != nil {
		t.Fatalf("FindBlank() failed: %v", err)
	}
	if blank != nil {
		t.Errorf("Expected no blank session initially, got %v", blank.ID)
	}

	// 2. Add an empty session
	sess, _ := store.Create()

	// Add it to directory list for store.List()
	fs.dirs[store.storageDir] = []os.DirEntry{
		&mockDirEntry{name: sess.ID, isDir: true},
	}

	blank, err = store.FindBlank()
	if err != nil {
		t.Fatalf("FindBlank() failed: %v", err)
	}
	if blank == nil {
		t.Fatal("Expected to find a blank session")
	}
	if blank.ID != sess.ID {
		t.Errorf("Expected to find session %s, got %s", sess.ID, blank.ID)
	}

	// 3. Add a message - should no longer be blank
	sess.Messages = append(sess.Messages, &schema.Message{Role: schema.User, Content: "hi"})
	_ = store.SaveSession(sess)

	blank, err = store.FindBlank()
	if err != nil {
		t.Fatalf("FindBlank() failed: %v", err)
	}
	if blank != nil {
		t.Errorf("Expected no blank session after adding message, got %v", blank.ID)
	}

	// 4. Add a name - should no longer be blank
	sess.Messages = []*schema.Message{}
	sess.Name = "Named Session"
	_ = store.SaveSession(sess)

	blank, err = store.FindBlank()
	if err != nil {
		t.Fatalf("FindBlank() failed: %v", err)
	}
	if blank != nil {
		t.Errorf("Expected no blank session after naming, got %v", blank.ID)
	}
}

func TestRename(t *testing.T) {
	store, _ := newTestStore()
	sess, _ := store.Create()

	err := store.Rename(sess.ID, "New Name")
	if err != nil {
		t.Fatalf("Rename() failed: %v", err)
	}

	// Load and verify
	loaded, _ := store.GetSession(sess.ID)
	if loaded.Name != "New Name" {
		t.Errorf("Expected name %q, got %q", "New Name", loaded.Name)
	}

	// Rename non-existent
	err = store.Rename("wrong-id", "Oops")
	if err == nil {
		t.Error("Rename() should have failed for non-existent session")
	}
}

func TestStore_LoadSaveChecksums(t *testing.T) {
	store, fs := newTestStore()
	sessID := "test-session-checksums"

	checksums := map[string]string{
		"/file1.txt": "hash1",
		"/file2.txt": "hash2",
	}

	// Save
	if err := store.SaveChecksums(sessID, checksums); err != nil {
		t.Fatalf("SaveChecksums() failed: %v", err)
	}

	// Verify it was written to file
	expectedPath := filepath.Join(store.storageDir, sessID, "checksums.json")
	data, ok := fs.files[expectedPath]
	if !ok {
		t.Fatalf("expected checksums.json to be written at %s", expectedPath)
	}

	var loaded map[string]string
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal written checksums: %v", err)
	}

	if loaded["/file1.txt"] != "hash1" || loaded["/file2.txt"] != "hash2" {
		t.Errorf("unexpected saved checksums content: %v", loaded)
	}

	// Load
	retrieved, err := store.LoadChecksums(sessID)
	if err != nil {
		t.Fatalf("LoadChecksums() failed: %v", err)
	}

	if retrieved["/file1.txt"] != "hash1" || retrieved["/file2.txt"] != "hash2" {
		t.Errorf("unexpected loaded checksums: %v", retrieved)
	}

	// Non-existent session load
	empty, err := store.LoadChecksums("non-existent-session-id")
	if err != nil {
		t.Fatalf("LoadChecksums should succeed for non-existent session, got: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected empty map for non-existent session, got %v", empty)
	}
}

func TestStore_SetActive(t *testing.T) {
	store, fs := newTestStore()

	// Create 2 sessions in the same directory, both inactive
	s1, err := store.Create()
	if err != nil {
		t.Fatalf("Create s1 failed: %v", err)
	}
	s2, err := store.Create()
	if err != nil {
		t.Fatalf("Create s2 failed: %v", err)
	}

	// Set both to same WorkingDir via get+modify+save
	s1.WorkingDir = "/dir"
	s1.Active = false
	if err := store.SaveSession(s1); err != nil {
		t.Fatalf("Save s1 failed: %v", err)
	}
	s2.WorkingDir = "/dir"
	s2.Active = false
	if err := store.SaveSession(s2); err != nil {
		t.Fatalf("Save s2 failed: %v", err)
	}

	// Make dirs visible to List()
	fs.dirs[store.storageDir] = []os.DirEntry{
		&mockDirEntry{name: s1.ID, isDir: true},
		&mockDirEntry{name: s2.ID, isDir: true},
	}

	// Activate s1 in /dir
	if err := store.SetActive(s1.ID, "/dir"); err != nil {
		t.Fatalf("SetActive(s1) failed: %v", err)
	}

	// Verify s1 is now active
	loaded, err := store.GetSession(s1.ID)
	if err != nil {
		t.Fatalf("Get s1 failed: %v", err)
	}
	if !loaded.Active {
		t.Error("s1 should be Active=true after SetActive")
	}
	if loaded.WorkingDir != "/dir" {
		t.Errorf("s1 WorkingDir should be /dir, got %s", loaded.WorkingDir)
	}

	// Verify s2 is still inactive
	loaded, err = store.GetSession(s2.ID)
	if err != nil {
		t.Fatalf("Get s2 failed: %v", err)
	}
	if loaded.Active {
		t.Error("s2 should still be Active=false after s1 activated")
	}

	// Activate s2 in /dir (should deactivate s1)
	if err := store.SetActive(s2.ID, "/dir"); err != nil {
		t.Fatalf("SetActive(s2) failed: %v", err)
	}

	// Verify s2 is now active
	loaded, err = store.GetSession(s2.ID)
	if err != nil {
		t.Fatalf("Get s2 failed: %v", err)
	}
	if !loaded.Active {
		t.Error("s2 should be Active=true after SetActive")
	}

	// Verify s1 is now inactive
	loaded, err = store.GetSession(s1.ID)
	if err != nil {
		t.Fatalf("Get s1 failed: %v", err)
	}
	if loaded.Active {
		t.Error("s1 should be Active=false after s2 activated")
	}

	// SetActive with nonexistent ID should return error
	err = store.SetActive("nonexistent", "/dir")
	if err == nil {
		t.Error("SetActive with nonexistent ID should return error")
	}
}

func TestStore_FindActiveForDir(t *testing.T) {
	store, fs := newTestStore()

	// Create 3 sessions and set Active flags manually
	s1, err := store.Create()
	if err != nil {
		t.Fatalf("Create s1 failed: %v", err)
	}
	s2, err := store.Create()
	if err != nil {
		t.Fatalf("Create s2 failed: %v", err)
	}
	s3, err := store.Create()
	if err != nil {
		t.Fatalf("Create s3 failed: %v", err)
	}

	// Manually set WorkingDir and Active via get+modify+save
	s1.WorkingDir = "/dir1"
	s1.Active = true
	if err := store.SaveSession(s1); err != nil {
		t.Fatalf("Save s1 failed: %v", err)
	}

	s2.WorkingDir = "/dir1"
	s2.Active = false
	if err := store.SaveSession(s2); err != nil {
		t.Fatalf("Save s2 failed: %v", err)
	}

	s3.WorkingDir = "/dir2"
	s3.Active = true
	if err := store.SaveSession(s3); err != nil {
		t.Fatalf("Save s3 failed: %v", err)
	}

	// Register all session dirs so List() can find them
	fs.dirs[store.storageDir] = []os.DirEntry{
		&mockDirEntry{name: s1.ID, isDir: true},
		&mockDirEntry{name: s2.ID, isDir: true},
		&mockDirEntry{name: s3.ID, isDir: true},
	}

	// FindActiveForDir /dir1 should return s1's summary
	summary, err := store.FindActiveForDir("/dir1")
	if err != nil {
		t.Fatalf("FindActiveForDir(/dir1) failed: %v", err)
	}
	if summary == nil {
		t.Fatal("FindActiveForDir(/dir1) should not return nil")
	}
	if summary.ID != s1.ID {
		t.Errorf("expected s1 (%s), got %s", s1.ID, summary.ID)
	}
	if !summary.Active {
		t.Error("summary should be Active=true")
	}
	if summary.WorkingDir != "/dir1" {
		t.Errorf("expected WorkingDir /dir1, got %s", summary.WorkingDir)
	}

	// FindActiveForDir /dir2 should return s3's summary
	summary, err = store.FindActiveForDir("/dir2")
	if err != nil {
		t.Fatalf("FindActiveForDir(/dir2) failed: %v", err)
	}
	if summary == nil {
		t.Fatal("FindActiveForDir(/dir2) should not return nil")
	}
	if summary.ID != s3.ID {
		t.Errorf("expected s3 (%s), got %s", s3.ID, summary.ID)
	}

	// FindActiveForDir /nonexistent should return nil
	summary, err = store.FindActiveForDir("/nonexistent")
	if err != nil {
		t.Fatalf("FindActiveForDir(/nonexistent) failed: %v", err)
	}
	if summary != nil {
		t.Errorf("expected nil for nonexistent dir, got %v", summary)
	}
}

func TestStore_ActiveFieldPreserved(t *testing.T) {
	store, fs := newTestStore()

	sess := &domain.Session{
		SessionMetadata: domain.SessionMetadata{
			ID:      "active-test",
			Name:    "Active Test",
			Created: time.Now(),
			Updated: time.Now(),
			Active:  true,
		},
		SessionMessages: domain.SessionMessages{Messages: []*schema.Message{}},
	}

	if err := store.SaveSession(sess); err != nil {
		t.Fatalf("SaveSession() failed: %v", err)
	}

	loaded, err := store.GetSession("active-test")
	if err != nil {
		t.Fatalf("GetSession() failed: %v", err)
	}
	if !loaded.Active {
		t.Error("Session.Active should be true after save/get roundtrip")
	}

	fs.dirs[store.storageDir] = []os.DirEntry{
		&mockDirEntry{name: "active-test", isDir: true},
	}
	summaries, err := store.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	if len(summaries) > 0 && !summaries[0].Active {
		t.Error("SessionSummary.Active should be true")
	}
}

func TestDefaultStorageDir(t *testing.T) {
	dir, err := DefaultStorageDir()
	if err != nil {
		t.Fatalf("DefaultStorageDir failed: %v", err)
	}
	if dir == "" {
		t.Error("expected non-empty storage directory")
	}
	if filepath.Base(dir) != "sessions" {
		t.Errorf("expected suffix sessions, got %s", dir)
	}
}



