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

type authBus interface {
	SendUIUpdate(domain.UIUpdate)
	WorkflowActions() <-chan domain.Action
}

// AuthDeps contains the dependencies for the auth workflow.
type AuthDeps struct {
	Bus      authBus
	Registry authRegistry
	AuthMgr  authManager
	State    authState
}

// RunAuth starts the authentication workflow asynchronously.
func RunAuth(ctx context.Context, deps *AuthDeps) <-chan error {
	done := make(chan error, 1)
	go func() {
		defer close(done)
		wf := NewAuthWorkflow(deps.Registry, deps.AuthMgr, deps.State)

		// 1. Initial snapshot
		snapshot, err := wf.Gather(ctx)
		if err != nil {
			done <- err
			return
		}
		deps.Bus.SendUIUpdate(*snapshot)

		var selectedProvider domain.Provider
		var selectedMethod domain.AuthMethod

		for {
			select {
			case <-ctx.Done():
				done <- ctx.Err()
				return
			case act, ok := <-deps.Bus.WorkflowActions():
				if !ok {
					done <- nil
					return
				}

				switch a := act.(type) {
				case domain.SelectProviderAction:
					p, ok := wf.registry.GetProvider(a.ID)
					if !ok {
						deps.Bus.SendUIUpdate(domain.AuthErrorEvent{Error: "Provider not found"})
						continue
					}
					selectedProvider = p
					deps.Bus.SendUIUpdate(domain.AuthMethodEvent{
						ProviderID: p.ID(),
						Methods:    p.SupportedAuthMethods(),
					})

				case domain.RemoveAuthAction:
					if err := wf.RemoveAuth(ctx, a.ProviderID); err != nil {
						deps.Bus.SendUIUpdate(domain.AuthErrorEvent{Error: err.Error()})
						continue
					}
					// Send refreshed snapshot
					snapshot, _ := wf.Gather(ctx)
					deps.Bus.SendUIUpdate(*snapshot)

				case domain.SelectAuthMethodAction:
					for _, m := range selectedProvider.SupportedAuthMethods() {
						if m.ID == a.ID {
							selectedMethod = m
							break
						}
					}
					deps.Bus.SendUIUpdate(domain.CredentialFieldEvent{
						Method:     selectedMethod,
						FieldIndex: 0,
					})

				case domain.SubmitCredentialAction:
					if err := wf.authMgr.Set(selectedProvider.ID(), a.Credential); err != nil {
						deps.Bus.SendUIUpdate(domain.AuthErrorEvent{Error: err.Error()})
						continue
					}
					deps.Bus.SendUIUpdate(domain.DoneEvent{})
					done <- nil
					return

				case domain.StopAction:
					deps.Bus.SendUIUpdate(domain.DoneEvent{})
					done <- nil
					return
				}
			}
		}
	}()
	return done
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
func (w *AuthWorkflow) Gather(ctx context.Context) (*domain.AuthProviderListEvent, error) {
	infos, err := w.registry.ListProviders(ctx)
	if err != nil {
		return nil, err
	}

	var results []domain.ProviderSummary
	for _, info := range infos {
		summary := domain.ProviderSummary{
			ID:         info.ID,
			Authorized: info.Credential != nil,
		}
		if info.Credential != nil {
			summary.AuthMethod = info.Credential.Type
		}
		results = append(results, summary)
	}

	return &domain.AuthProviderListEvent{
		Providers: results,
	}, nil
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
