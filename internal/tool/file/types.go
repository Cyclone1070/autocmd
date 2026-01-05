package file

import (
	"fmt"

	"github.com/Cyclone1070/iav/internal/config"
)

// -- Write File --

type WriteFileRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (r *WriteFileRequest) Validate(cfg *config.Config) error {
	if r.Path == "" {
		return fmt.Errorf("path is required")
	}
	if r.Content == "" {
		return fmt.Errorf("content is required")
	}
	if int64(len(r.Content)) > cfg.Tools.MaxFileSize {
		return fmt.Errorf("content too large: %d bytes exceeds limit %d", len(r.Content), cfg.Tools.MaxFileSize)
	}
	return nil
}

type WriteFileResponse struct {
	AbsolutePath string
	RelativePath string
	BytesWritten int
}

func (r WriteFileResponse) Success() bool {
	return true
}
