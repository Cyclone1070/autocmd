package workflow

import (
	"context"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
)

type authRegistry interface {
	ListProviders(ctx context.Context) ([]domain.ProviderInfo, error)
	GetProvider(id string) (domain.Provider, bool)
}

type authManager interface {
	Set(providerID string, cred domain.Credential) error
	Remove(providerID string) error
}

type authState interface {
	Model() string
	SetModel(id string)
	Save() error
}

// AuthWorkflow orchestrates authentication operations.
type AuthWorkflow struct {
	registry authRegistry
	authMgr  authManager
	state    authState
}

// NewAuthWorkflow creates a new AuthWorkflow.
func NewAuthWorkflow(registry authRegistry, authMgr authManager, state authState) *AuthWorkflow {
	return &AuthWorkflow{
		registry: registry,
		authMgr:  authMgr,
		state:    state,
	}
}

// Gather returns the providers and their authentication status.
func (w *AuthWorkflow) Gather(ctx context.Context) (*domain.AuthWorkflowResult, error) {
	infos, err := w.registry.ListProviders(ctx)
	if err != nil {
		return nil, err
	}

	var results []domain.ProviderSummary
	for _, info := range infos {
		results = append(results, domain.ProviderSummary{
			ID:         info.ID,
			Authorized: info.Credential != nil,
		})
	}

	return &domain.AuthWorkflowResult{
		Providers: results,
	}, nil
}

// SetAuth sets the authentication credentials for a provider.
func (w *AuthWorkflow) SetAuth(ctx context.Context, providerID string, cred domain.Credential) error {
	return w.authMgr.Set(providerID, cred)
}

// RemoveAuth removes the authentication credentials for a provider and resets active model if needed.
func (w *AuthWorkflow) RemoveAuth(ctx context.Context, providerID string) error {
	if err := w.authMgr.Remove(providerID); err != nil {
		return err
	}

	// Reset current model if it belongs to the provider we just removed
	if strings.HasPrefix(w.state.Model(), providerID+"/") {
		w.state.SetModel("")
		return w.state.Save()
	}

	return nil
}

// GetProvider returns a provider from the registry.
func (w *AuthWorkflow) GetProvider(id string) (domain.Provider, bool) {
	return w.registry.GetProvider(id)
}
