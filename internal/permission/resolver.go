// Package permission provides a mechanism for authorizing tool executions based on user-defined policies.
package permission

// Mode is the effective policy decision for a tool call.
type Mode string

const (
	// ModeAsk requires explicit user approval before each tool execution.
	ModeAsk Mode = "ask"
	// ModeAllow permits tool execution without further prompts.
	ModeAllow Mode = "allow"
	// ModeDeny blocks tool execution unconditionally.
	ModeDeny Mode = "deny"
)

// Resolver returns effective permission mode for tools.
type Resolver struct {
	byTool      map[string]Mode
	defaultMode Mode
}

// NewResolver builds a resolver from config values.
// Invalid values are normalized to ask as a safe fallback.
func NewResolver(defaultMode string, byTool map[string]string) *Resolver {
	resolver := &Resolver{
		defaultMode: parseMode(defaultMode),
		byTool:      make(map[string]Mode, len(byTool)),
	}
	for tool, mode := range byTool {
		resolver.byTool[tool] = parseMode(mode)
	}
	return resolver
}

// Resolve returns the effective permission mode for a specific tool.
func (r *Resolver) Resolve(toolName string) Mode {
	if r == nil {
		return ModeAllow
	}
	if mode, ok := r.byTool[toolName]; ok {
		return mode
	}
	return r.defaultMode
}

func parseMode(mode string) Mode {
	switch Mode(mode) {
	case ModeAllow:
		return ModeAllow
	case ModeDeny:
		return ModeDeny
	default:
		return ModeAsk
	}
}
