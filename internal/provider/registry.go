package provider

import (
	"sort"

	"github.com/Cyclone1070/iav/internal/domain"
)

// Registry holds LLM provider instances and provides lookup.
type Registry struct {
	providers map[string]domain.Provider
}

// NewRegistry creates a Registry from a list of providers.
func NewRegistry(providers []domain.Provider) *Registry {
	r := &Registry{
		providers: make(map[string]domain.Provider),
	}
	for _, p := range providers {
		r.providers[p.Name()] = p
	}
	return r
}

// Get returns a provider by name.
func (r *Registry) Get(name string) (domain.Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// List returns all registered provider names in sorted order.
func (r *Registry) List() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
