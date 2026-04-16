package domain

import "context"

type contextKey int

const (
	sessionIDKey contextKey = iota
)

// WithSessionID returns a new context with the session ID attached.
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionIDKey, sessionID)
}

// GetSessionID returns the session ID from the context if it exists.
func GetSessionID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(sessionIDKey).(string)
	return id, ok
}
