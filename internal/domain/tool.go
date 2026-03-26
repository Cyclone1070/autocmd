package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/cloudwego/eino/schema"
)


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
	Comment   string `json:"comment"` // User-friendly description (e.g. "Updating auth")
	Target    string `json:"target"`  // Technical action (e.g. "Edit auth.go")
	Added     int    `json:"added"`   // Lines added
	Removed   int    `json:"removed"` // Lines removed
	Diff      string `json:"diff"`    // Unified diff content
}

func (DiffDisplay) isToolDisplay() {}
func (d DiffDisplay) Type() string { return d.TypeField }

// NewDiffDisplay creates a new DiffDisplay with correct type.
func NewDiffDisplay(comment, target string, added, removed int, diff string) DiffDisplay {
	return DiffDisplay{
		TypeField: "diff",
		Comment:   comment,
		Target:    target,
		Added:     added,
		Removed:   removed,
		Diff:      diff,
	}
}

// ShellDisplay is for shell command execution with streaming output.
type ShellDisplay struct {
	TypeField      string    `json:"type"`
	Comment        string    `json:"comment"`         // Description from tool (e.g. "Installing dependencies")
	Command        string    `json:"command"`         // The command being run (e.g. "npm install")
	CapturedOutput *string   `json:"captured_output"` // Pointer to raw output captured after execution (baked)
	Output         io.Reader `json:"-"`               // Stream stdout/stderr (transient)
}

func (ShellDisplay) isToolDisplay() {}
func (s ShellDisplay) Type() string { return s.TypeField }

// NewShellDisplay creates a new ShellDisplay with correct type.
func NewShellDisplay(comment, command string, output io.Reader, capturedOutput *string) ShellDisplay {
	return ShellDisplay{
		TypeField:      "shell",
		Comment:        comment,
		Command:        command,
		Output:         output,
		CapturedOutput: capturedOutput,
	}
}

// ToolDisplays is a helper type for polymorphic JSON unmarshaling of ToolDisplay maps.
type ToolDisplays map[string]ToolDisplay

func (m *ToolDisplays) UnmarshalJSON(data []byte) error {
	var raws map[string]json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return err
	}

	*m = make(ToolDisplays)
	for id, raw := range raws {
		var peek struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &peek); err != nil {
			return err
		}

		var display ToolDisplay
		switch peek.Type {
		case "string":
			var d StringDisplay
			if err := json.Unmarshal(raw, &d); err != nil {
				return err
			}
			display = d
		case "diff":
			var d DiffDisplay
			if err := json.Unmarshal(raw, &d); err != nil {
				return err
			}
			display = d
		case "shell":
			var d ShellDisplay
			if err := json.Unmarshal(raw, &d); err != nil {
				return err
			}
			display = d
		default:
			return fmt.Errorf("unknown display type: %s", peek.Type)
		}
		(*m)[id] = display
	}
	return nil
}

// Tool defines the interface for individual tools.
type Tool interface {
	Name() string
	Definition() *schema.ToolInfo
	Prepare(ctx context.Context, params string) (Invocation, error)
}
