package tool

import (
	"context"
	"io"
)

// Type represents JSON Schema types.
type Type string

const (
	TypeString  Type = "string"
	TypeNumber  Type = "number"
	TypeInteger Type = "integer"
	TypeBoolean Type = "boolean"
	TypeArray   Type = "array"
	TypeObject  Type = "object"
)

// Schema represents a JSON Schema for tool parameters.
type Schema struct {
	Type        Type               `json:"type"`
	Description string             `json:"description,omitempty"`
	Properties  map[string]*Schema `json:"properties,omitempty"`
	Required    []string           `json:"required,omitempty"`
	Items       *Schema            `json:"items,omitempty"`
	Enum        []string           `json:"enum,omitempty"`
}

// Declaration declares a tool's function signature for the LLM.
type Declaration struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Parameters  *Schema `json:"parameters,omitempty"`
}

// Invocation is a validated, prepared tool call ready for execution.
// Returned by Tool.Prepare(), enforces prepare-before-execute sequence.
type Invocation interface {
	// Display returns what to show in UI (computed during Prepare).
	Display() ToolDisplay

	// Execute runs the operation and returns content for the LLM.
	// Success: (content, nil)
	// Failure: (errorContent, err) - errorContent shown to LLM, err signals failure.
	Execute(ctx context.Context) (llmContent string, err error)
}

// ToolDisplay is implemented by all display types returned from tools.
// The UI uses type switches to render each type appropriately.
type ToolDisplay interface {
	isToolDisplay()
}

// StringDisplay is for simple text output (most tools).
type StringDisplay string

func (StringDisplay) isToolDisplay() {}

// DiffDisplay is for file edit operations with unified diff content.
type DiffDisplay struct {
	Filename     string // File being edited
	Diff         string // Unified diff content
	AddedLines   int
	RemovedLines int
}

func (DiffDisplay) isToolDisplay() {}

// ShellDisplay is for shell command execution with streaming output.
type ShellDisplay struct {
	Command    string
	WorkingDir string
	Output     io.Reader // Stream stdout/stderr from here
	Wait       func()    // Call after reading Output to wait for execution to finish.
}

func (ShellDisplay) isToolDisplay() {}
