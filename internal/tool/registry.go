package tool

import (
	"sort"

	"github.com/Cyclone1070/iav/internal/domain"
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

// Declarations returns all tool declarations for the LLM, sorted by name.
func (r *Registry) Declarations() []domain.Declaration {
	decls := make([]domain.Declaration, 0, len(r.tools))
	for _, t := range r.tools {
		decls = append(decls, t.Declaration())
	}
	sort.Slice(decls, func(i, j int) bool {
		return decls[i].Name < decls[j].Name
	})
	return decls
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (domain.Tool, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}
