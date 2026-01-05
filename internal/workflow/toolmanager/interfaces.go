package toolmanager

import (
	"context"
	"encoding/json"

	"github.com/Cyclone1070/iav/internal/tool"
)

// Invocation is a validated, prepared tool call ready for execution.
// Returned by Tool.Prepare(), enforces prepare-before-execute sequence.
type Invocation interface {
	// Display returns what to show in UI (computed during Prepare).
	// Returns rich display types: DiffDisplay for edit, StringDisplay for read, etc.
	Display() tool.ToolDisplay

	// Execute runs the operation and returns content for the LLM.
	// Success: (content, nil)
	// Failure: (errorContent, err) - errorContent shown to LLM, err signals failure.
	// User sees generic "execution failed", LLM sees detailed error in content.
	Execute(ctx context.Context) (llmContent string, err error)
}

// Tool defines the interface for individual tools.
type Tool interface {
	// Name returns the tool's identifier.
	Name() string

	// Declaration returns the tool's schema for the LLM.
	Declaration() tool.Declaration

	// Prepare unmarshals params, validates comprehensively, and returns an Invocation.
	// Handles: JSON parsing, path validation, file existence, snippet matching, etc.
	// If Prepare succeeds, Execute should only fail due to write errors or race conditions.
	Prepare(ctx context.Context, params json.RawMessage) (Invocation, error)
}
