package workflow

import (
	"context"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockAuthProvider struct {
	mock.Mock
}

func (m *mockAuthProvider) ID() string { return m.Called().String(0) }
func (m *mockAuthProvider) Name() string { return m.Called().String(0) }
func (m *mockAuthProvider) SupportedAuthMethods() []domain.AuthMethod {
	return m.Called().Get(0).([]domain.AuthMethod)
}
func (m *mockAuthProvider) Models() []domain.LLMInfo {
	return m.Called().Get(0).([]domain.LLMInfo)
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

func TestAuthWorkflow(t *testing.T) {
	ctx := context.Background()

	t.Run("Gather returns providers and status", func(t *testing.T) {
		registry := new(mockAuthRegistry)
		authMgr := new(mockAuthManager)
		state := new(mockAuthState)

		infos := []domain.ProviderInfo{
			{ID: "openai", Credential: &domain.Credential{}},
			{ID: "anthropic", Credential: nil},
		}
		registry.On("ListProviders", mock.Anything).Return(infos, nil)

		wf := NewAuthWorkflow(registry, authMgr, state)
		res, err := wf.Gather(ctx)

		assert.NoError(t, err)
		assert.Len(t, res.Providers, 2)
		assert.True(t, res.Providers[0].Authorized)
		assert.False(t, res.Providers[1].Authorized)
		registry.AssertExpectations(t)
	})

	t.Run("SetAuth saves credential", func(t *testing.T) {
		registry := new(mockAuthRegistry)
		authMgr := new(mockAuthManager)
		state := new(mockAuthState)

		cred := domain.Credential{Type: "api_key", APIKey: "secret"}
		authMgr.On("Set", "openai", cred).Return(nil)

		wf := NewAuthWorkflow(registry, authMgr, state)
		err := wf.SetAuth(ctx, "openai", cred)

		assert.NoError(t, err)
		authMgr.AssertExpectations(t)
	})

	t.Run("RemoveAuth clears model if it belongs to provider", func(t *testing.T) {
		registry := new(mockAuthRegistry)
		authMgr := new(mockAuthManager)
		state := new(mockAuthState)

		authMgr.On("Remove", "openai").Return(nil)
		state.On("Model").Return("openai/gpt-4")
		state.On("SetModel", "").Return()
		state.On("Save").Return(nil)

		wf := NewAuthWorkflow(registry, authMgr, state)
		err := wf.RemoveAuth(ctx, "openai")

		assert.NoError(t, err)
		authMgr.AssertExpectations(t)
		state.AssertExpectations(t)
	})

	t.Run("RemoveAuth does not clear model if it belongs to different provider", func(t *testing.T) {
		registry := new(mockAuthRegistry)
		authMgr := new(mockAuthManager)
		state := new(mockAuthState)

		authMgr.On("Remove", "openai").Return(nil)
		state.On("Model").Return("anthropic/claude-3")

		wf := NewAuthWorkflow(registry, authMgr, state)
		err := wf.RemoveAuth(ctx, "openai")

		assert.NoError(t, err)
		authMgr.AssertExpectations(t)
		state.AssertNotCalled(t, "SetModel", mock.Anything)
	})
}
