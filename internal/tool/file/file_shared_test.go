package file

import (
	"fmt"
	"path/filepath"
	"strings"

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

