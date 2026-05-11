// Package agent provides the core reasoning loop and tool execution logic.
package agent

import (
	"errors"
	"strings"
)

var (
	// ErrModelBackend marks failures originating from model transport/stream/provider layers.
	ErrModelBackend = errors.New("model backend failure")
	// ErrModelAuth marks model/provider authentication failures (invalid/expired credentials).
	ErrModelAuth = errors.New("model authentication failure")
)

func classifyModelErr(err error) error {
	if err == nil {
		return ErrModelBackend
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "unauthorized") ||
		strings.Contains(s, "unauthenticated") ||
		strings.Contains(s, "permission_denied") ||
		strings.Contains(s, "permission denied") ||
		strings.Contains(s, "invalid api key") ||
		strings.Contains(s, "api key not valid") ||
		strings.Contains(s, "invalid authentication") ||
		strings.Contains(s, "status: unauthenticated") ||
		strings.Contains(s, "status: permission_denied") ||
		strings.Contains(s, "error 401") ||
		strings.Contains(s, "error 403") {
		return ErrModelAuth
	}
	return ErrModelBackend
}
