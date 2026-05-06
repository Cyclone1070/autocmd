// Package tool provides the tool registry and core tool implementations.
package tool

import (
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
		n, ok := t.(interface{ Name() string })
		if !ok {
			continue
		}
		name := n.Name()
		registry.tools[name] = t
	}
	return registry
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (einotool.BaseTool, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

func (r *Registry) Tools() []einotool.BaseTool {
	tools := make([]einotool.BaseTool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}
