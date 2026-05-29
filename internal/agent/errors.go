// Package agent provides the core reasoning loop and tool execution logic.
package agent

import (
	"errors"
)

// ErrModel failure originating from model transport/stream/provider layers.
var ErrModel = errors.New("model failure")
