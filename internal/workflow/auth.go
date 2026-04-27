package workflow

import (
	"context"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
)

type authRegistry interface {
	List(ctx context.Context) ([]domain.ProviderInfo, error)
	Get(id string) (domain.Provider, bool)
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
	OAuthMgr oauthManager
	State    authState
}

type oauthManager interface {
	RunDeviceFlow(ctx context.Context, cfg domain.OAuthMethod, onCode func(string, string)) (string, error)
}

// RunAuth starts the authentication workflow asynchronously.
func RunAuth(ctx context.Context, deps *AuthDeps) <-chan error {
	done := make(chan error, 1)
	go func() {
		defer close(done)
		wf := NewAuthWorkflow(deps.Registry, deps.AuthMgr, deps.OAuthMgr, deps.State)

		// 1. Initial snapshot
		snapshot, err := wf.Gather(ctx)
		if err != nil {
			done <- err
			return
		}
		deps.Bus.SendUIUpdate(*snapshot)

		var selectedProvider domain.Provider
		var selectedMethod domain.AuthMethod
		var oauthCancel context.CancelFunc

		for {
			select {
			case <-ctx.Done():
				if oauthCancel != nil {
					oauthCancel()
				}
				done <- ctx.Err()
				return
			case act, ok := <-deps.Bus.WorkflowActions():
				if !ok {
					done <- nil
					return
				}

			processAction:
				switch a := act.(type) {
				case domain.SelectProviderAction:
					p, ok := wf.registry.Get(a.ID)
					if !ok {
						deps.Bus.SendUIUpdate(domain.AuthErrorEvent{Error: "Provider not found"})
						continue
					}
					selectedProvider = p

					var interactive []domain.AuthMethod
					var envVars []string

					for _, m := range p.SupportedAuthMethods() {
						switch v := m.(type) {
						case domain.EnvVarAuthMethod:
							envVars = append(envVars, v.EnvVars...)
						default:
							interactive = append(interactive, m)
						}
					}

					if len(interactive) == 0 {
						if len(envVars) > 0 {
							deps.Bus.SendUIUpdate(domain.EnvVarInstructionEvent{EnvVars: envVars})
						} else {
							deps.Bus.SendUIUpdate(domain.AuthErrorEvent{Error: "Provider has no configuration options available"})
						}
						deps.Bus.SendUIUpdate(domain.DoneEvent{})
						done <- nil
						return
					}

					deps.Bus.SendUIUpdate(domain.AuthMethodEvent{
						ProviderID: p.ID(),
						Methods:    interactive,
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
						var id string
						switch v := m.(type) {
						case domain.APIKeyAuthMethod:
							id = v.ID
						case domain.OAuthMethod:
							id = v.ID
						case domain.EnvVarAuthMethod:
							id = v.ID
						}
						if id == a.ID {
							selectedMethod = m
							break
						}
					}
					if selectedMethod == nil {
						continue
					}

					switch v := selectedMethod.(type) {
					case domain.APIKeyAuthMethod:
						deps.Bus.SendUIUpdate(domain.CredentialFieldEvent{
							Method:     selectedMethod,
							FieldIndex: 0,
						})
					case domain.OAuthMethod:
						// Run device flow asynchronously so StopAction can be handled promptly.
						if wf.oauthMgr == nil {
							deps.Bus.SendUIUpdate(domain.AuthErrorEvent{Error: "OAuth manager not configured"})
							continue
						}

						oauthCtx, cancel := context.WithCancel(ctx)
						oauthCancel = cancel
						resultCh := make(chan struct {
							token string
							err   error
						}, 1)

						go func() {
							token, err := wf.oauthMgr.RunDeviceFlow(oauthCtx, v, func(uri, code string) {
								deps.Bus.SendUIUpdate(domain.OAuthDeviceFlowEvent{
									VerificationURI: uri,
									UserCode:        code,
								})
							})
							resultCh <- struct {
								token string
								err   error
							}{token: token, err: err}
						}()

						select {
						case <-ctx.Done():
							cancel()
							done <- ctx.Err()
							return
						case act2, ok := <-deps.Bus.WorkflowActions():
							if !ok {
								cancel()
								done <- nil
								return
							}
							if _, isStop := act2.(domain.StopAction); isStop {
								cancel()
								deps.Bus.SendUIUpdate(domain.DoneEvent{})
								done <- nil
								return
							}
							// User changed direction while OAuth was in progress.
							// Cancel OAuth and process the new action immediately.
							cancel()
							act = act2
							goto processAction
						case res := <-resultCh:
							oauthCancel = nil
							if res.err != nil {
								deps.Bus.SendUIUpdate(domain.AuthErrorEvent{Error: res.err.Error()})
								continue
							}

							cred := domain.Credential{Type: v.ID, OAuthToken: res.token}
							wf.authMgr.Set(selectedProvider.ID(), cred)
							deps.Bus.SendUIUpdate(domain.DoneEvent{})
							done <- nil
							return
						}
					}

				case domain.SubmitCredentialAction:
					if err := wf.authMgr.Set(selectedProvider.ID(), a.Credential); err != nil {
						deps.Bus.SendUIUpdate(domain.AuthErrorEvent{Error: err.Error()})
						continue
					}
					deps.Bus.SendUIUpdate(domain.DoneEvent{})
					done <- nil
					return

				case domain.StopAction:
					if oauthCancel != nil {
						oauthCancel()
					}
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
	oauthMgr oauthManager
	state    authState
}

// NewAuthWorkflow creates a new AuthWorkflow.
func NewAuthWorkflow(registry authRegistry, authMgr authManager, oauthMgr oauthManager, state authState) *AuthWorkflow {
	return &AuthWorkflow{
		registry: registry,
		authMgr:  authMgr,
		oauthMgr: oauthMgr,
		state:    state,
	}
}

// Gather returns the providers and their authentication status.
func (w *AuthWorkflow) Gather(ctx context.Context) (*domain.AuthProviderListEvent, error) {
	infos, err := w.registry.List(ctx)
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
func (w *AuthWorkflow) RemoveAuth(_ context.Context, providerID string) error {
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
	return w.registry.Get(id)
}
