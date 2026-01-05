package file

import (
	"fmt"
	"path/filepath"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/tool"
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

// -- Edit File --

type EditOperation struct {
	Before               string `json:"before"`
	After                string `json:"after"`
	ExpectedReplacements int    `json:"expected_replacements,omitempty"`
}

type EditFileRequest struct {
	Path       string          `json:"path"`
	Operations []EditOperation `json:"operations"`
}

func (r *EditFileRequest) Display() string {
	return filepath.Base(r.Path)
}

func (r *EditFileRequest) Validate() error {
	if r.Path == "" {
		return fmt.Errorf("path is required")
	}
	if len(r.Operations) == 0 {
		return fmt.Errorf("operations are required")
	}
	for i := range r.Operations {
		if r.Operations[i].ExpectedReplacements <= 0 {
			r.Operations[i].ExpectedReplacements = 1
		}
	}
	return nil
}

type EditFileResponse struct {
	Path  string // File path for success message
	Error string // Set if the tool failed

	// For DiffDisplay
	Diff         string // Unified diff content
	AddedLines   int
	RemovedLines int
}

// LLMContent returns success message or error
func (r *EditFileResponse) LLMContent() string {
	if r.Error != "" {
		return fmt.Sprintf("Error: %s", r.Error)
	}
	return fmt.Sprintf("Successfully modified file: %s", r.Path)
}

// Display returns DiffDisplay for UI rendering
func (r *EditFileResponse) Display() tool.ToolDisplay {
	if r.Error != "" {
		return tool.StringDisplay("Bad request")
	}
	return tool.DiffDisplay{
		Diff:         r.Diff,
		AddedLines:   r.AddedLines,
		RemovedLines: r.RemovedLines,
	}
}

func (r EditFileResponse) Success() bool {
	return r.Error == ""
}
