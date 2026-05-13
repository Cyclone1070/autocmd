package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
)

const modelIDParts = 2

// CredentialStore defines the interface for active credential resolution.
type CredentialStore interface {
	GetWithFallback(p domain.Provider) (*domain.Credential, error)
}

// Registry manages the set of available LLM providers.
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

// Get returns a provider by its ID.
func (r *Registry) Get(id string) (domain.Provider, bool) {
	p, ok := r.providers[id]
	return p, ok
}

// List returns information about all registered providers, including resolved credentials.
func (r *Registry) List(_ context.Context) ([]domain.ProviderInfo, error) {
	infos := make([]domain.ProviderInfo, 0, len(r.sortedProviderIDs()))
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

func (r *Registry) sortedProviderIDs() []string {
	ids := make([]string, 0, len(r.providers))
	for id := range r.providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// LLMRegistry orchestrates model resolution using a Registry and credentials.
type LLMRegistry struct {
	providers   *Registry
	authManager CredentialStore
}

// NewLLMRegistry creates an LLMRegistry.
func NewLLMRegistry(authManager CredentialStore, providers *Registry) *LLMRegistry {
	return &LLMRegistry{
		providers:   providers,
		authManager: authManager,
	}
}

// Get resolves "google/gemini-2.5-flash" to an LLM instance.
func (r *LLMRegistry) Get(ctx context.Context, id string) (domain.LLM, error) {
	parts := strings.SplitN(id, modelIDSeparator, modelIDParts)
	if len(parts) != modelIDParts {
		return nil, fmt.Errorf("invalid LLM ID format: %s (expected provider/model)", id)
	}

	pID := parts[0]
	mID := parts[1]

	p, ok := r.providers.Get(pID)
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

	if !hasValidCredential(cred) {
		return nil, fmt.Errorf("no valid credential found for %s", pID)
	}

	// Find the model info from the provider's list
	var modelInfo *domain.LLMInfo
	for _, m := range p.List() {
		if m.ID == id {
			modelInfo = &m
			break
		}
	}

	if modelInfo == nil {
		return nil, fmt.Errorf("model %s not found in provider %s", mID, pID)
	}

	return p.GetLLM(ctx, cred, *modelInfo)
}

// List returns all LLMs from all providers.
func (r *LLMRegistry) List(_ context.Context) ([]domain.LLMInfo, error) {
	var all []domain.LLMInfo
	for _, id := range r.providers.sortedProviderIDs() {
		p, _ := r.providers.Get(id)
		var cred *domain.Credential
		if r.authManager != nil {
			resolved, _ := r.authManager.GetWithFallback(p)
			cred = resolved
		}

		if !hasValidCredential(cred) {
			continue
		}

		llms := p.List()
		all = append(all, llms...)
	}
	return all, nil
}

func hasValidCredential(cred *domain.Credential) bool {
	if cred == nil {
		return false
	}
	return cred.APIKey != "" || cred.OAuthToken != ""
}
