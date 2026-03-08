package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
)

// Registry resolves LLM IDs to LLM instances using provider-specific credentials.
type Registry struct {
	providers map[string]domain.Provider
}

// NewRegistry creates a Registry from providers.
func NewRegistry(providers ...domain.Provider) *Registry {
	r := &Registry{
		providers: make(map[string]domain.Provider),
	}
	for _, p := range providers {
		if p != nil {
			r.providers[p.ID()] = p
		}
	}
	return r
}

// GetProvider returns a provider by its ID.
func (r *Registry) GetProvider(id string) (domain.Provider, bool) {
	p, ok := r.providers[id]
	return p, ok
}

// ListProviders returns the IDs of all registered providers.
func (r *Registry) ListProviders() []string {
	var ids []string
	for id := range r.providers {
		ids = append(ids, id)
	}
	return ids
}

// Get resolves "google/gemini-2.5-flash" to an LLM using the provided credential.
func (r *Registry) Get(ctx context.Context, id string, cred *domain.Credential) (domain.LLM, error) {
	parts := strings.SplitN(id, domain.ModelIDSeparator, 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid LLM ID format: %s (expected provider/model)", id)
	}
	p, ok := r.providers[parts[0]]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", parts[0])
	}
	return p.GetLLM(ctx, cred, parts[1])
}

// List returns all LLMs from all providers. Only returns models for providers with valid credentials.
func (r *Registry) List(ctx context.Context, creds map[string]*domain.Credential) ([]domain.LLMInfo, error) {
	var all []domain.LLMInfo
	for id, p := range r.providers {
		cred := creds[id]
		if cred == nil || ((cred.Type == domain.AuthMethodAPIKey || cred.Type == domain.AuthMethodEnv) && cred.APIKey == "") {
			continue
		}

		llms, err := p.ListLLMs(ctx, cred)
		if err != nil {
			return nil, fmt.Errorf("list LLMs from %s: %w", id, err)
		}
		for _, m := range llms {
			all = append(all, domain.LLMInfo{
				ID:          id + domain.ModelIDSeparator + m.ID,
				DisplayName: m.DisplayName,
			})
		}
	}
	return all, nil
}
