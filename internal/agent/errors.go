// Package agent provides the core reasoning loop and tool execution logic.
package agent

import (
	"errors"
)

var (
	// ErrModel failure originating from model transport/stream/provider layers.
	ErrModel = errors.New("model failure")
)
