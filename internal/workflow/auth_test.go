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

func (m *mockAuthRegistry) ListProviders(ctx context.Context) ([]domain.ProviderInfo, error) {
	args := m.Called(ctx)
	return args.Get(0).([]domain.ProviderInfo), args.Error(1)
}

func (m *mockAuthRegistry) GetProvider(id string) (domain.Provider, bool) {
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

func (m *mockAuthState) Model() string { return m.Called().String(0) }
func (m *mockAuthState) SetModel(id string) { m.Called(id) }
func (m *mockAuthState) Save() error { return m.Called().Error(0) }

type mockProvider struct {
	mock.Mock
}

func (m *mockProvider) ID() string   { return m.Called().String(0) }
func (m *mockProvider) Name() string { return m.Called().String(0) }
func (m *mockProvider) SupportedAuthMethods() []domain.AuthMethod {
	return m.Called().Get(0).([]domain.AuthMethod)
}
func (m *mockProvider) ListLLMs() []domain.LLMInfo { return nil }
func (m *mockProvider) GetLLM(ctx context.Context, cred *domain.Credential, modelID string) (domain.LLM, error) {
	return nil, nil
}

func TestRunAuth(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	registry := new(mockAuthRegistry)
	authMgr := new(mockAuthManager)
	state := new(mockAuthState)
	bus := new(mockAuthBus)
	actions := make(chan domain.Action, 10)
	bus.On("WorkflowActions").Return((<-chan domain.Action)(actions))

	t.Run("Full Auth Flow", func(t *testing.T) {
		// 1. Initial Load
		registry.On("ListProviders", mock.Anything).Return([]domain.ProviderInfo{{ID: "openai"}}, nil)
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
		methods := []domain.AuthMethod{{ID: "api_key", Label: "API Key", Fields: []domain.AuthField{{ID: "key"}}}}
		p.On("SupportedAuthMethods").Return(methods)
		registry.On("GetProvider", "openai").Return(p, true)
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
		registry.On("ListProviders", mock.Anything).Return([]domain.ProviderInfo{}, nil)
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
		registry.On("ListProviders", mock.Anything).Return(infos, nil)

		wf := NewAuthWorkflow(registry, authMgr, state)
		snapshot, err := wf.Gather(ctx)

		assert.NoError(t, err)
		assert.Equal(t, "api_key", snapshot.Providers[0].AuthMethod)
		assert.Empty(t, snapshot.Providers[1].AuthMethod)
	})
}
