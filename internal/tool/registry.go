package tool

import (
	"sort"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
)

// Registry holds tool instances and provides lookup.
type Registry struct {
	tools map[string]domain.Tool
}

// NewRegistry creates a Registry from pre-built tools.
func NewRegistry(tools []domain.Tool) *Registry {
	registry := &Registry{tools: make(map[string]domain.Tool)}
	for _, t := range tools {
		registry.tools[t.Name()] = t
	}
	return registry
}

// Definitions returns all tool definitions for the LLM, sorted by name.
func (r *Registry) Definitions() []*schema.ToolInfo {
	defs := make([]*schema.ToolInfo, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, t.Definition())
	}
	sort.Slice(defs, func(i, j int) bool {
		return defs[i].Name < defs[j].Name
	})
	return defs
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (domain.Tool, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}
