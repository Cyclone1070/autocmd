package file

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stretchr/testify/mock"
)

type mockPathResolver struct {
	mock.Mock
	workspaceRoot string
}

func (m *mockPathResolver) Abs(p string) (string, error) {
	if strings.Contains(p, "..") {
		return "", fmt.Errorf("path %s is outside workspace", p)
	}
	if filepath.IsAbs(p) {
		return p, nil
	}
	return filepath.Join(m.workspaceRoot, p), nil
}

func (m *mockPathResolver) DisplayPath(p string) string {
	rel, err := filepath.Rel(m.workspaceRoot, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(p)
	}
	if rel == "." {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(rel)
}

func (m *mockPathResolver) Root() string {
	return m.workspaceRoot
}

type toolMockFileInfo struct {
	name  string
	size  int64
	isDir bool
}

func (m toolMockFileInfo) Name() string       { return m.name }
func (m toolMockFileInfo) Size() int64        { return m.size }
func (m toolMockFileInfo) Mode() os.FileMode  { return 0o644 }
func (m toolMockFileInfo) ModTime() time.Time { return time.Time{} }
func (m toolMockFileInfo) IsDir() bool        { return m.isDir }
func (m toolMockFileInfo) Sys() any           { return nil }

type mockFileOps struct {
	files map[string][]byte
	dirs  map[string]bool
}

func newMockFileOps() *mockFileOps {
	return &mockFileOps{
		files: make(map[string][]byte),
		dirs:  make(map[string]bool),
	}
}

func (m *mockFileOps) ReadFile(path string) ([]byte, error) {
	if m.dirs[path] {
		return nil, fmt.Errorf("is a directory")
	}
	if c, ok := m.files[path]; ok {
		return c, nil
	}
	return nil, os.ErrNotExist
}

func (m *mockFileOps) Stat(path string) (os.FileInfo, error) {
	if m.dirs[path] {
		return toolMockFileInfo{name: path, isDir: true}, nil
	}
	if c, ok := m.files[path]; ok {
		return toolMockFileInfo{name: path, size: int64(len(c)), isDir: false}, nil
	}
	return nil, os.ErrNotExist
}

func (m *mockFileOps) WriteFileAtomic(path string, content []byte, perm os.FileMode) error {
	m.files[path] = content
	return nil
}

func (m *mockFileOps) EnsureDirs(path string) error {
	dir := filepath.Dir(path)
	m.dirs[dir] = true
	return nil
}

type mockChecksumManagerShared struct {
	checksums map[string]string
}

func newMockChecksumManagerShared() *mockChecksumManagerShared {
	return &mockChecksumManagerShared{
		checksums: make(map[string]string),
	}
}

func (m *mockChecksumManagerShared) Compute(data []byte) string {
	return fmt.Sprintf("checksum-%d", len(data))
}

func (m *mockChecksumManagerShared) Get(path string) (string, bool) {
	c, ok := m.checksums[path]
	return c, ok
}

func (m *mockChecksumManagerShared) Update(path string, checksum string) {
	m.checksums[path] = checksum
}
