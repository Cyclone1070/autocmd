// Package workflow implements the core business logic and state transitions for various system operations.
package workflow

import (
	"context"
	"strings"

	"github.com/Cyclone1070/autocmd/internal/domain"
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
		session := &authSession{
			deps: deps,
			wf:   NewAuthWorkflow(deps.Registry, deps.AuthMgr, deps.OAuthMgr, deps.State),
		}

		// 1. Initial snapshot
		snapshot, err := session.wf.Gather(ctx)
		if err != nil {
			done <- err
			return
		}
		deps.Bus.SendUIUpdate(*snapshot)

		for {
			select {
			case <-ctx.Done():
				session.cancelOAuth()
				done <- ctx.Err()
				return
			case act, ok := <-deps.Bus.WorkflowActions():
				if !ok {
					done <- nil
					return
				}

				for act != nil {
					var next domain.Action
					var finished bool
					_, next, finished, err = session.processAction(ctx, act)
					if finished {
						done <- err
						return
					}
					act = next
				}
			}
		}
	}()
	return done
}

type authSession struct {
	deps             *AuthDeps
	wf               *AuthWorkflow
	selectedProvider domain.Provider
	selectedMethod   domain.AuthMethod
	oauthCancel      context.CancelFunc
}

func (s *authSession) cancelOAuth() {
	if s.oauthCancel != nil {
		s.oauthCancel()
		s.oauthCancel = nil
	}
}

func (s *authSession) processAction(ctx context.Context, act domain.Action) (curr, next domain.Action, finished bool, err error) {
	switch a := act.(type) {
	case domain.SelectProviderAction:
		return nil, nil, s.handleSelectProvider(a), nil
	case domain.RemoveAuthAction:
		s.handleRemoveAuth(ctx, a)
	case domain.SelectAuthMethodAction:
		return s.handleSelectAuthMethod(ctx, a)
	case domain.SubmitCredentialAction:
		return nil, nil, true, s.handleSubmitCredential(a)
	case domain.StopAction:
		s.cancelOAuth()
		s.deps.Bus.SendUIUpdate(domain.DoneEvent{})
		return nil, nil, true, nil
	}
	return nil, nil, false, nil
}

func (s *authSession) handleSelectProvider(a domain.SelectProviderAction) (finished bool) {
	p, ok := s.wf.registry.Get(a.ID)
	if !ok {
		s.deps.Bus.SendUIUpdate(domain.AuthErrorEvent{Error: "Provider not found"})
		return false
	}
	s.selectedProvider = p

	var interactive []domain.AuthMethod
	var envVars []string

	for _, m := range p.SupportedAuthMethods() {
		if env, ok := m.(domain.EnvVarAuthMethod); ok {
			envVars = append(envVars, env.EnvVars...)
		} else {
			interactive = append(interactive, m)
		}
	}

	if len(interactive) == 0 {
		if len(envVars) > 0 {
			s.deps.Bus.SendUIUpdate(domain.EnvVarInstructionEvent{EnvVars: envVars})
		} else {
			s.deps.Bus.SendUIUpdate(domain.AuthErrorEvent{Error: "Provider has no configuration options available"})
		}
		s.deps.Bus.SendUIUpdate(domain.DoneEvent{})
		return true
	}

	s.deps.Bus.SendUIUpdate(domain.AuthMethodEvent{
		ProviderID: p.ID(),
		Methods:    interactive,
	})
	return false
}

func (s *authSession) handleRemoveAuth(ctx context.Context, a domain.RemoveAuthAction) {
	if err := s.wf.RemoveAuth(ctx, a.ProviderID); err != nil {
		s.deps.Bus.SendUIUpdate(domain.AuthErrorEvent{Error: err.Error()})
		return
	}
	snapshot, _ := s.wf.Gather(ctx)
	s.deps.Bus.SendUIUpdate(*snapshot)
}

func (s *authSession) handleSelectAuthMethod(ctx context.Context, a domain.SelectAuthMethodAction) (curr, next domain.Action, finished bool, err error) {
	for _, m := range s.selectedProvider.SupportedAuthMethods() {
		if getAuthMethodID(m) == a.ID {
			s.selectedMethod = m
			break
		}
	}
	if s.selectedMethod == nil {
		return nil, nil, false, nil
	}

	switch v := s.selectedMethod.(type) {
	case domain.APIKeyAuthMethod:
		s.deps.Bus.SendUIUpdate(domain.CredentialFieldEvent{
			Method:     s.selectedMethod,
			FieldIndex: 0,
		})
		return nil, nil, false, nil
	case domain.OAuthMethod:
		return s.runOAuthFlow(ctx, v)
	}
	return nil, nil, false, nil
}

func (s *authSession) runOAuthFlow(ctx context.Context, method domain.OAuthMethod) (curr, next domain.Action, finished bool, err error) {
	if s.wf.oauthMgr == nil {
		s.deps.Bus.SendUIUpdate(domain.AuthErrorEvent{Error: "OAuth manager not configured"})
		return nil, nil, false, nil
	}

	oauthCtx, cancel := context.WithCancel(ctx)
	s.oauthCancel = cancel
	defer s.cancelOAuth()

	type result struct {
		err   error
		token string
	}
	resultCh := make(chan result, 1)

	go func() {
		token, err := s.wf.oauthMgr.RunDeviceFlow(oauthCtx, method, func(uri, code string) {
			s.deps.Bus.SendUIUpdate(domain.OAuthDeviceFlowEvent{
				VerificationURI: uri,
				UserCode:        code,
			})
		})
		resultCh <- result{token: token, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, nil, true, ctx.Err()
	case act, ok := <-s.deps.Bus.WorkflowActions():
		if !ok {
			return nil, nil, true, nil
		}
		if _, isStop := act.(domain.StopAction); isStop {
			s.deps.Bus.SendUIUpdate(domain.DoneEvent{})
			return nil, nil, true, nil
		}
		// Interrupt: process the new action
		return nil, act, false, nil
	case res := <-resultCh:
		if res.err != nil {
			s.deps.Bus.SendUIUpdate(domain.AuthErrorEvent{Error: res.err.Error()})
			return nil, nil, false, nil
		}
		cred := domain.Credential{Type: method.ID, OAuthToken: res.token}
		if err := s.wf.authMgr.Set(s.selectedProvider.ID(), cred); err != nil {
			s.deps.Bus.SendUIUpdate(domain.AuthErrorEvent{Error: err.Error()})
			return nil, nil, false, nil
		}
		s.deps.Bus.SendUIUpdate(domain.DoneEvent{})
		return nil, nil, true, nil
	}
}

func (s *authSession) handleSubmitCredential(a domain.SubmitCredentialAction) error {
	if err := s.wf.authMgr.Set(s.selectedProvider.ID(), a.Credential); err != nil {
		s.deps.Bus.SendUIUpdate(domain.AuthErrorEvent{Error: err.Error()})
		return err
	}
	s.deps.Bus.SendUIUpdate(domain.DoneEvent{})
	return nil
}

func getAuthMethodID(m domain.AuthMethod) string {
	switch v := m.(type) {
	case domain.APIKeyAuthMethod:
		return v.ID
	case domain.OAuthMethod:
		return v.ID
	case domain.EnvVarAuthMethod:
		return v.ID
	default:
		return ""
	}
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
