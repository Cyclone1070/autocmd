package domain

import (
	"context"
	"encoding/json"
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
// Must be in domain so tools can implement without importing workflow.
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
// Must be in domain so tools can implement without importing workflow.
type ToolDisplay interface {
	Type() string
	isToolDisplay()
}

// StringDisplay is for simple text output (most tools).
type StringDisplay struct {
	TypeField string `json:"type"`
	Content   string `json:"content"`
}

func (StringDisplay) isToolDisplay() {}
func (s StringDisplay) Type() string { return s.TypeField }

// NewStringDisplay creates a new StringDisplay with correct type.
func NewStringDisplay(content string) StringDisplay {
	return StringDisplay{TypeField: "string", Content: content}
}

// DiffDisplay is for file edit operations with unified diff content.
type DiffDisplay struct {
	TypeField string `json:"type"`
	Header    string `json:"header"`  // e.g. "Edit config.yaml"
	Added     int    `json:"added"`   // Lines added
	Removed   int    `json:"removed"` // Lines removed
	Diff      string `json:"diff"`    // Unified diff content
}

func (DiffDisplay) isToolDisplay() {}
func (d DiffDisplay) Type() string { return d.TypeField }

// NewDiffDisplay creates a new DiffDisplay with correct type.
func NewDiffDisplay(header string, added, removed int, diff string) DiffDisplay {
	return DiffDisplay{
		TypeField: "diff",
		Header:    header,
		Added:     added,
		Removed:   removed,
		Diff:      diff,
	}
}

// ShellDisplay is for shell command execution with streaming output.
type ShellDisplay struct {
	TypeField      string    `json:"type"`
	Header         string    `json:"header"`          // Description from tool (e.g. "Installing dependencies")
	Command        string    `json:"command"`         // The command being run (e.g. "npm install")
	CapturedOutput *string   `json:"captured_output"` // Pointer to raw output captured after execution (baked)
	Output         io.Reader `json:"-"`               // Stream stdout/stderr (transient)
	Wait           func()    `json:"-"`               // Wait for execution to finish (transient)
}

func (ShellDisplay) isToolDisplay() {}
func (s ShellDisplay) Type() string { return s.TypeField }

// NewShellDisplay creates a new ShellDisplay with correct type.
func NewShellDisplay(header, command string, output io.Reader, wait func()) ShellDisplay {
	return ShellDisplay{
		TypeField:      "shell",
		Header:         header,
		Command:        command,
		Output:         output,
		Wait:           wait,
		CapturedOutput: nil,
	}
}

// Tool defines the interface for individual tools.
type Tool interface {
	Name() string
	Declaration() Declaration
	Prepare(ctx context.Context, params json.RawMessage) (Invocation, error)
}
