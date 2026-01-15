package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
)

// provider is the private contract for backends.
type provider interface {
	Name() string
	ListModels(ctx context.Context) ([]domain.ModelInfo, error)
	GetModel(ctx context.Context, id string) (domain.Model, error)
}

// Registry resolves model IDs to Model instances.
type Registry struct {
	providers map[string]provider
}

// NewRegistry creates a Registry from providers.
func NewRegistry(providers ...provider) *Registry {
	r := &Registry{
		providers: make(map[string]provider),
	}
	for _, p := range providers {
		if p != nil {
			r.providers[p.Name()] = p
		}
	}
	return r
}

// Get resolves "google/gemini-2.5-flash" to a Model.
func (r *Registry) Get(ctx context.Context, id string) (domain.Model, error) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid model ID format: %s (expected provider/model)", id)
	}
	p, ok := r.providers[parts[0]]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", parts[0])
	}
	return p.GetModel(ctx, parts[1])
}

// List returns all models from all providers.
func (r *Registry) List(ctx context.Context) ([]domain.ModelInfo, error) {
	var all []domain.ModelInfo
	for name, p := range r.providers {
		models, err := p.ListModels(ctx)
		if err != nil {
			return nil, fmt.Errorf("list models from %s: %w", name, err)
		}
		for _, m := range models {
			all = append(all, domain.ModelInfo{
				ID:          name + "/" + m.ID,
				DisplayName: m.DisplayName,
			})
		}
	}
	return all, nil
}
