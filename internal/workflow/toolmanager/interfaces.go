package toolmanager

import (
	"context"
	"encoding/json"

	"github.com/Cyclone1070/iav/internal/tool"
)

// Tool defines the interface for individual tools.
type Tool interface {
	// Name returns the tool's identifier.
	Name() string

	// Declaration returns the tool's schema for the LLM.
	Declaration() tool.Declaration

	// Prepare unmarshals params, validates comprehensively, and returns an Invocation.
	// Handles: JSON parsing, path validation, file existence, snippet matching, etc.
	// If Prepare succeeds, Execute should only fail due to write errors or race conditions.
	Prepare(ctx context.Context, params json.RawMessage) (tool.Invocation, error)
}
