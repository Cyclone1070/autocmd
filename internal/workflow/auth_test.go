package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockAuthBus struct {
	mock.Mock
}

func (m *mockAuthBus) SendUIUpdate(update domain.UIUpdate) {
	m.Called(update)
}

func (m *mockAuthBus) WorkflowActions() <-chan domain.Action {
	args := m.Called()
	return args.Get(0).(<-chan domain.Action)
}

type mockAuthRegistry struct {
	mock.Mock
}

func (m *mockAuthRegistry) List(ctx context.Context) ([]domain.ProviderInfo, error) {
	args := m.Called(ctx)
	return args.Get(0).([]domain.ProviderInfo), args.Error(1)
}

func (m *mockAuthRegistry) Get(id string) (domain.Provider, bool) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, false
	}
	return args.Get(0).(domain.Provider), args.Bool(1)
}

type mockAuthManager struct {
	mock.Mock
}

func (m *mockAuthManager) Set(providerID string, cred domain.Credential) error {
	return m.Called(providerID, cred).Error(0)
}

func (m *mockAuthManager) Remove(providerID string) error {
	return m.Called(providerID).Error(0)
}

type mockAuthState struct {
	mock.Mock
}

func (m *mockAuthState) Model() string      { return m.Called().String(0) }
func (m *mockAuthState) SetModel(id string) { m.Called(id) }
func (m *mockAuthState) Save() error        { return m.Called().Error(0) }

type mockProvider struct {
	mock.Mock
}

func (m *mockProvider) ID() string   { return m.Called().String(0) }
func (m *mockProvider) Name() string { return m.Called().String(0) }
func (m *mockProvider) SupportedAuthMethods() []domain.AuthMethod {
	return m.Called().Get(0).([]domain.AuthMethod)
}
func (m *mockProvider) List() []domain.LLMInfo { return nil }
func (m *mockProvider) GetLLM(_ context.Context, _ *domain.Credential, _ domain.LLMInfo) (domain.LLM, error) {
	return nil, nil
}

func TestRunAuth(t *testing.T) {
	ctx := t.Context()

	registry := new(mockAuthRegistry)
	authMgr := new(mockAuthManager)
	state := new(mockAuthState)
	bus := new(mockAuthBus)
	actions := make(chan domain.Action, 10)
	bus.On("WorkflowActions").Return((<-chan domain.Action)(actions))

	t.Run("Full Auth Flow", func(t *testing.T) {
		// 1. Initial Load
		registry.On("List", mock.Anything).Return([]domain.ProviderInfo{{ID: "openai"}}, nil)
		bus.On("SendUIUpdate", mock.MatchedBy(func(ev domain.UIUpdate) bool {
			snapshot, ok := ev.(domain.AuthProviderListEvent)
			return ok && len(snapshot.Providers) == 1
		})).Return()

		done := RunAuth(ctx, &AuthDeps{
			Bus:      bus,
			Registry: registry,
			AuthMgr:  authMgr,
			State:    state,
		})

		// 2. Select Provider
		p := new(mockProvider)
		p.On("ID").Return("openai")
		methods := []domain.AuthMethod{
			domain.APIKeyAuthMethod{ID: "api_key", Name: "API Key", Fields: []domain.AuthField{{ID: "key"}}},
		}
		p.On("SupportedAuthMethods").Return(methods)
		registry.On("Get", "openai").Return(p, true)
		bus.On("SendUIUpdate", domain.AuthMethodEvent{ProviderID: "openai", Methods: methods}).Return()

		actions <- domain.SelectProviderAction{ID: "openai"}

		// 3. Select Method
		bus.On("SendUIUpdate", domain.CredentialFieldEvent{Method: methods[0], FieldIndex: 0}).Return()
		actions <- domain.SelectAuthMethodAction{ID: "api_key"}

		// 4. Submit Credential
		cred := domain.Credential{Type: "api_key", APIKey: "secret"}
		authMgr.On("Set", "openai", cred).Return(nil)
		bus.On("SendUIUpdate", domain.DoneEvent{}).Return()

		actions <- domain.SubmitCredentialAction{Credential: cred}

		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("workflow timed out")
		}

		bus.AssertExpectations(t)
		registry.AssertExpectations(t)
		authMgr.AssertExpectations(t)
	})

	t.Run("StopAction triggers DoneEvent and exits", func(t *testing.T) {
		registry.On("List", mock.Anything).Return([]domain.ProviderInfo{}, nil)
		bus.On("SendUIUpdate", mock.AnythingOfType("domain.AuthProviderListEvent")).Return()
		bus.On("SendUIUpdate", domain.DoneEvent{}).Return()

		done := RunAuth(ctx, &AuthDeps{
			Bus:      bus,
			Registry: registry,
			AuthMgr:  authMgr,
			State:    state,
		})

		actions <- domain.StopAction{}

		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("workflow timed out on StopAction")
		}
	})

	t.Run("Gather populates AuthMethod", func(t *testing.T) {
		registry := new(mockAuthRegistry)
		authMgr := new(mockAuthManager)
		state := new(mockAuthState)

		infos := []domain.ProviderInfo{
			{ID: "openai", Credential: &domain.Credential{Type: "api_key"}},
			{ID: "anthropic", Credential: nil},
		}
		registry.On("List", mock.Anything).Return(infos, nil)

		wf := NewAuthWorkflow(registry, authMgr, nil, state)
		snapshot, err := wf.Gather(ctx)

		assert.NoError(t, err)
		assert.Equal(t, "api_key", snapshot.Providers[0].AuthMethod)
		assert.Empty(t, snapshot.Providers[1].AuthMethod)
	})

	t.Run("EnvVar Fallback Only", func(t *testing.T) {
		registry := new(mockAuthRegistry)
		authMgr := new(mockAuthManager)
		state := new(mockAuthState)
		bus := new(mockAuthBus)
		actions := make(chan domain.Action, 10)
		bus.On("WorkflowActions").Return((<-chan domain.Action)(actions))

		// 1. Initial Load
		registry.On("List", mock.Anything).Return([]domain.ProviderInfo{{ID: "google"}}, nil)
		bus.On("SendUIUpdate", mock.AnythingOfType("domain.AuthProviderListEvent")).Return()

		done := RunAuth(ctx, &AuthDeps{
			Bus:      bus,
			Registry: registry,
			AuthMgr:  authMgr,
			State:    state,
		})

		// 2. Select Provider
		p := new(mockProvider)
		p.On("ID").Return("google")
		methods := []domain.AuthMethod{
			domain.EnvVarAuthMethod{ID: "env", Name: "Env", EnvVars: []string{"GEMINI_API_KEY"}},
		}
		p.On("SupportedAuthMethods").Return(methods)
		registry.On("Get", "google").Return(p, true)

		// Expect Instruction Event
		bus.On("SendUIUpdate", domain.EnvVarInstructionEvent{EnvVars: []string{"GEMINI_API_KEY"}}).Return()
		bus.On("SendUIUpdate", domain.DoneEvent{}).Return()

		actions <- domain.SelectProviderAction{ID: "google"}

		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(time.Second * 5):
			t.Fatal("workflow timed out")
		}

		bus.AssertExpectations(t)
		registry.AssertExpectations(t)
	})

	t.Run("OAuth Flow", func(t *testing.T) {
		registry := new(mockAuthRegistry)
		authMgr := new(mockAuthManager)
		oauthMgr := new(mockOAuthManager)
		state := new(mockAuthState)
		bus := new(mockAuthBus)
		actions := make(chan domain.Action, 10)
		bus.On("WorkflowActions").Return((<-chan domain.Action)(actions))

		// 1. Initial Load
		registry.On("List", mock.Anything).Return([]domain.ProviderInfo{{ID: "github"}}, nil)
		bus.On("SendUIUpdate", mock.AnythingOfType("domain.AuthProviderListEvent")).Return()

		done := RunAuth(ctx, &AuthDeps{
			Bus:      bus,
			Registry: registry,
			AuthMgr:  authMgr,
			OAuthMgr: oauthMgr,
			State:    state,
		})

		// 2. Select Provider
		p := new(mockProvider)
		p.On("ID").Return("github")
		oauthMethod := domain.OAuthMethod{ID: "github_oauth", Name: "GitHub"}
		methods := []domain.AuthMethod{oauthMethod}
		p.On("SupportedAuthMethods").Return(methods)
		registry.On("Get", "github").Return(p, true)
		bus.On("SendUIUpdate", domain.AuthMethodEvent{ProviderID: "github", Methods: methods}).Return()

		actions <- domain.SelectProviderAction{ID: "github"}

		// 3. Select OAuth Method
		oauthMgr.On("RunDeviceFlow", mock.Anything, oauthMethod, mock.Anything).Run(func(args mock.Arguments) {
			onCode := args.Get(2).(func(string, string))
			onCode("https://github.com/login/device", "CODE-123")
		}).Return("gho_test_token", nil)

		bus.On("SendUIUpdate", domain.OAuthDeviceFlowEvent{
			VerificationURI: "https://github.com/login/device",
			UserCode:        "CODE-123",
		}).Return()

		// #nosec G101
		authMgr.On("Set", "github", domain.Credential{Type: "github_oauth", OAuthToken: "gho_test_token"}).Return(nil)
		bus.On("SendUIUpdate", domain.DoneEvent{}).Return()

		actions <- domain.SelectAuthMethodAction{ID: "github_oauth"}

		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(time.Second * 5):
			t.Fatal("workflow timed out")
		}

		bus.AssertExpectations(t)
		oauthMgr.AssertExpectations(t)
		authMgr.AssertExpectations(t)
	})

	t.Run("OAuth Flow StopAction cancels promptly", func(t *testing.T) {
		registry := new(mockAuthRegistry)
		authMgr := new(mockAuthManager)
		oauthMgr := new(mockOAuthManager)
		state := new(mockAuthState)
		bus := new(mockAuthBus)
		actions := make(chan domain.Action, 10)
		bus.On("WorkflowActions").Return((<-chan domain.Action)(actions))

		// 1. Initial Load
		registry.On("List", mock.Anything).Return([]domain.ProviderInfo{{ID: "github"}}, nil)
		bus.On("SendUIUpdate", mock.AnythingOfType("domain.AuthProviderListEvent")).Return()

		ctx := t.Context()

		done := RunAuth(ctx, &AuthDeps{
			Bus:      bus,
			Registry: registry,
			AuthMgr:  authMgr,
			OAuthMgr: oauthMgr,
			State:    state,
		})

		// 2. Select Provider
		p := new(mockProvider)
		p.On("ID").Return("github")
		oauthMethod := domain.OAuthMethod{ID: "github_oauth", Name: "GitHub"}
		methods := []domain.AuthMethod{oauthMethod}
		p.On("SupportedAuthMethods").Return(methods)
		registry.On("Get", "github").Return(p, true)
		bus.On("SendUIUpdate", domain.AuthMethodEvent{ProviderID: "github", Methods: methods}).Return()

		actions <- domain.SelectProviderAction{ID: "github"}

		// 3. Select OAuth Method: block forever unless ctx is cancelled.
		blocked := make(chan struct{})
		oauthMgr.On("RunDeviceFlow", mock.Anything, oauthMethod, mock.Anything).Return("", context.Canceled).Run(func(_ mock.Arguments) {
			<-blocked
		})

		// Workflow should still emit DoneEvent on StopAction without waiting for device flow to finish.
		bus.On("SendUIUpdate", domain.DoneEvent{}).Return()

		actions <- domain.SelectAuthMethodAction{ID: "github_oauth"}
		actions <- domain.StopAction{}

		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("workflow timed out on StopAction during OAuth flow")
		}
	})

	t.Run("OAuth Flow selecting another provider cancels and processes action", func(t *testing.T) {
		registry := new(mockAuthRegistry)
		authMgr := new(mockAuthManager)
		oauthMgr := new(mockOAuthManager)
		state := new(mockAuthState)
		bus := new(mockAuthBus)
		actions := make(chan domain.Action, 10)
		bus.On("WorkflowActions").Return((<-chan domain.Action)(actions))

		// Initial provider list.
		registry.On("List", mock.Anything).Return([]domain.ProviderInfo{{ID: "github"}, {ID: "google"}}, nil)
		bus.On("SendUIUpdate", mock.AnythingOfType("domain.AuthProviderListEvent")).Return()

		done := RunAuth(ctx, &AuthDeps{
			Bus:      bus,
			Registry: registry,
			AuthMgr:  authMgr,
			OAuthMgr: oauthMgr,
			State:    state,
		})

		// Provider 1: github with OAuth method.
		githubProvider := new(mockProvider)
		githubProvider.On("ID").Return("github")
		githubOAuthMethod := domain.OAuthMethod{ID: "github_oauth", Name: "GitHub"}
		githubProvider.On("SupportedAuthMethods").Return([]domain.AuthMethod{githubOAuthMethod})
		registry.On("Get", "github").Return(githubProvider, true)
		bus.On("SendUIUpdate", domain.AuthMethodEvent{
			ProviderID: "github",
			Methods:    []domain.AuthMethod{githubOAuthMethod},
		}).Return()

		// Provider 2: google with API key method.
		googleProvider := new(mockProvider)
		googleProvider.On("ID").Return("google")
		googleAPIKeyMethod := domain.APIKeyAuthMethod{
			ID: "api_key", Name: "API Key", Fields: []domain.AuthField{{ID: "api_key"}},
		}
		googleProvider.On("SupportedAuthMethods").Return([]domain.AuthMethod{googleAPIKeyMethod})
		registry.On("Get", "google").Return(googleProvider, true)
		bus.On("SendUIUpdate", domain.AuthMethodEvent{
			ProviderID: "google",
			Methods:    []domain.AuthMethod{googleAPIKeyMethod},
		}).Return()

		// OAuth run stays blocked until cancelled.
		started := make(chan struct{})
		oauthMgr.On("RunDeviceFlow", mock.Anything, githubOAuthMethod, mock.Anything).Return("", context.Canceled).Run(func(args mock.Arguments) {
			close(started)
			<-args.Get(0).(context.Context).Done()
		})

		actions <- domain.SelectProviderAction{ID: "github"}
		actions <- domain.SelectAuthMethodAction{ID: "github_oauth"}

		// Ensure it started before we send the next action that would cancel it.
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("oauth flow did not start")
		}

		actions <- domain.SelectProviderAction{ID: "google"}

		// Stop workflow after asserting it kept progressing.
		bus.On("SendUIUpdate", domain.DoneEvent{}).Return()
		actions <- domain.StopAction{}

		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("workflow timed out while switching providers during OAuth")
		}

		bus.AssertExpectations(t)
		registry.AssertExpectations(t)
		oauthMgr.AssertExpectations(t)
	})
}

type mockOAuthManager struct {
	mock.Mock
}

func (m *mockOAuthManager) RunDeviceFlow(ctx context.Context, cfg domain.OAuthMethod, onCode func(string, string)) (string, error) {
	args := m.Called(ctx, cfg, onCode)
	return args.String(0), args.Error(1)
}
