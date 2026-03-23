package llm

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
)

// CredentialStore defines the interface for active credential resolution.
type CredentialStore interface {
	GetWithFallback(p domain.Provider) (*domain.Credential, error)
}

// Registry resolves LLM IDs to LLM instances using provider-specific credentials.
type Registry struct {
	providers   map[string]domain.Provider
	authManager CredentialStore
}

// NewRegistry creates a Registry from providers and an optional auth manager.
func NewRegistry(authManager CredentialStore, providers ...domain.Provider) *Registry {
	r := &Registry{
		providers:   make(map[string]domain.Provider),
		authManager: authManager,
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

// ListProviders returns information about all registered providers, including resolved credentials.
func (r *Registry) ListProviders(ctx context.Context) ([]domain.ProviderInfo, error) {
	var infos []domain.ProviderInfo
	for _, id := range r.sortedProviderIDs() {
		p := r.providers[id]
		cred := (*domain.Credential)(nil)
		if r.authManager != nil {
			resolved, _ := r.authManager.GetWithFallback(p)
			cred = resolved
		}
		infos = append(infos, domain.ProviderInfo{
			ID:         id,
			Credential: cred,
		})
	}
	return infos, nil
}

// Get resolves "google/gemini-2.5-flash" to an LLM.
// It tries to resolve it using the internal auth manager.
func (r *Registry) Get(ctx context.Context, id string) (domain.LLM, error) {
	parts := strings.SplitN(id, domain.ModelIDSeparator, 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid LLM ID format: %s (expected provider/model)", id)
	}

	pID := parts[0]
	mID := parts[1]

	p, ok := r.providers[pID]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", pID)
	}

	// Active Resolution
	var cred *domain.Credential
	if r.authManager != nil {
		resolved, err := r.authManager.GetWithFallback(p)
		if err != nil {
			return nil, fmt.Errorf("resolve credential for %s: %w", pID, err)
		}
		cred = resolved
	}

	if cred == nil {
		return nil, fmt.Errorf("no credential provided or found for %s", pID)
	}

	return p.GetLLM(ctx, cred, mID)
}

// List returns all LLMs from all providers.
// It tries to resolve credentials for all providers using the internal auth manager.
func (r *Registry) List(ctx context.Context) ([]domain.LLMInfo, error) {
	var all []domain.LLMInfo
	for _, id := range r.sortedProviderIDs() {
		p := r.providers[id]
		var cred *domain.Credential
		if r.authManager != nil {
			resolved, _ := r.authManager.GetWithFallback(p)
			cred = resolved
		}

		if cred == nil || ((cred.Type == domain.AuthMethodAPIKey || cred.Type == domain.AuthMethodEnv) && cred.APIKey == "") {
			continue
		}

		llms := p.ListLLMs()
		for _, m := range llms {
			all = append(all, domain.LLMInfo{
				ID:          id + domain.ModelIDSeparator + m.ID,
				DisplayName: m.DisplayName,
			})
		}
	}
	return all, nil
}

func (r *Registry) sortedProviderIDs() []string {
	ids := make([]string, 0, len(r.providers))
	for id := range r.providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
