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
	ListLLMs(ctx context.Context) ([]domain.LLMInfo, error)
	GetLLM(ctx context.Context, id string) (domain.LLM, error)
}

// Registry resolves LLM IDs to LLM instances.
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

// Get resolves "google/gemini-2.5-flash" to an LLM.
func (r *Registry) Get(ctx context.Context, id string) (domain.LLM, error) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid LLM ID format: %s (expected provider/model)", id)
	}
	p, ok := r.providers[parts[0]]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", parts[0])
	}
	return p.GetLLM(ctx, parts[1])
}

// List returns all LLMs from all providers.
func (r *Registry) List(ctx context.Context) ([]domain.LLMInfo, error) {
	var all []domain.LLMInfo
	for name, p := range r.providers {
		llms, err := p.ListLLMs(ctx)
		if err != nil {
			return nil, fmt.Errorf("list LLMs from %s: %w", name, err)
		}
		for _, m := range llms {
			all = append(all, domain.LLMInfo{
				ID:          name + "/" + m.ID,
				DisplayName: m.DisplayName,
			})
		}
	}
	return all, nil
}
