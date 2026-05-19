// Package tool provides the tool registry and core tool implementations.
package tool

import (
	"context"
	"sort"

	einotool "github.com/cloudwego/eino/components/tool"
)

// Registry holds tool instances and provides lookup.
type Registry struct {
	tools map[string]einotool.BaseTool
}

// NewRegistry creates a Registry from pre-built tools.
func NewRegistry(tools []einotool.BaseTool) *Registry {
	registry := &Registry{
		tools: make(map[string]einotool.BaseTool),
	}
	for _, t := range tools {
		var name string
		if n, ok := t.(interface{ Name() string }); ok {
			name = n.Name()
		} else {
			info, err := t.Info(context.Background())
			if err != nil {
				continue
			}
			name = info.Name
		}
		if name != "" {
			registry.tools[name] = t
		}
	}
	return registry
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (einotool.BaseTool, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

// Tools returns the registered tools in a deterministic, name-sorted order.
//
// This ordering matters: the slice feeds straight into the LLM request payload
// (function declarations / tool schemas). Go's map iteration order is
// randomised per call, which would cause every request to send tools in a
// different order. That breaks the model's prompt KV cache for tool
// definitions, hurts output stability, and in practice causes thinking-capable
// Gemini models to flake hard (long TTFB and Error 500). Keep the order
// stable across calls.
func (r *Registry) Tools() []einotool.BaseTool {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	tools := make([]einotool.BaseTool, 0, len(r.tools))
	for _, name := range names {
		tools = append(tools, r.tools[name])
	}
	return tools
}
