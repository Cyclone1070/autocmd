// Package agent provides the core reasoning loop and tool execution logic.
package agent

import "errors"

var (
	// ErrModelBackend marks failures originating from model transport/stream/provider layers.
	ErrModelBackend = errors.New("model backend failure")
	// ErrModelAuth marks model/provider authentication failures (invalid/expired credentials).
	ErrModelAuth = errors.New("model authentication failure")
)
