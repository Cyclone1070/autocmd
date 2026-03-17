package workflow

import (
	"context"
	"fmt"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type infoMockLLMRegistry struct {
	mock.Mock
}

func (m *infoMockLLMRegistry) ListProviders(ctx context.Context) ([]domain.ProviderInfo, error) {
	args := m.Called(ctx)
	return args.Get(0).([]domain.ProviderInfo), args.Error(1)
}

func (m *infoMockLLMRegistry) Get(ctx context.Context, id string) (domain.LLM, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(domain.LLM), args.Error(1)
}

type infoMockState struct {
	mock.Mock
}

func (m *infoMockState) Model() string {
	args := m.Called()
	return args.String(0)
}

func (m *infoMockState) CurrentSessionID() string {
	args := m.Called()
	return args.String(0)
}

type infoMockSessionStore struct {
	mock.Mock
}

func (m *infoMockSessionStore) Get(id string) (*domain.Session, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Session), args.Error(1)
}

type infoMockLLM struct {
	mock.Mock
}

func (m *infoMockLLM) ID() string                    { return m.Called().String(0) }
func (m *infoMockLLM) DisplayName() string           { return m.Called().String(0) }
func (m *infoMockLLM) ContextWindow() int            { return m.Called().Int(0) }
func (m *infoMockLLM) ComputeTokens(ctx context.Context, msgs domain.Messages) (int, error) {
	args := m.Called(ctx, msgs)
	return args.Int(0), args.Error(1)
}
func (m *infoMockLLM) Stream(ctx context.Context, msgs domain.Messages, tools []domain.Declaration) (domain.Stream, error) {
	return nil, nil
}

func TestInfoWorkflow_Run(t *testing.T) {
	ctx := context.Background()

	t.Run("Full Success Scenario", func(t *testing.T) {
		registry := new(infoMockLLMRegistry)
		state := new(infoMockState)
		store := new(infoMockSessionStore)
		llm := new(infoMockLLM)

		registry.On("ListProviders", ctx).Return([]domain.ProviderInfo{
			{ID: "google", Credential: &domain.Credential{Type: domain.AuthMethodEnv}},
			{ID: "openai", Credential: nil},
		}, nil)

		state.On("Model").Return("google/gemini-pro")
		state.On("CurrentSessionID").Return("sess-123")

		session := &domain.Session{
			ID:   "sess-123",
			Name: "Test Session",
			Messages: domain.Messages{
				domain.UserMessage{Content: "Hello"},
			},
		}
		store.On("Get", "sess-123").Return(session, nil)

		registry.On("Get", ctx, "google/gemini-pro").Return(llm, nil)
		llm.On("ContextWindow").Return(128000)
		llm.On("ComputeTokens", ctx, session.Messages).Return(100, nil)

		wf := NewInfoWorkflow(registry, state, store)
		res, err := wf.Gather(ctx)

		assert.NoError(t, err)
		assert.Equal(t, "google/gemini-pro", res.Model)
		assert.Equal(t, "Test Session", res.SessionDisplay)
		assert.Equal(t, 100, res.SessionTokens)
		assert.Equal(t, 128000, res.ContextWindow)
		assert.ElementsMatch(t, []string{"google (env)"}, res.Authorized)
	})

	t.Run("No Model or Session", func(t *testing.T) {
		registry := new(infoMockLLMRegistry)
		state := new(infoMockState)
		store := new(infoMockSessionStore)

		registry.On("ListProviders", ctx).Return([]domain.ProviderInfo{}, nil)
		state.On("Model").Return("")
		state.On("CurrentSessionID").Return("")

		wf := NewInfoWorkflow(registry, state, store)
		res, err := wf.Gather(ctx)

		assert.NoError(t, err)
		assert.Equal(t, "", res.Model)
		assert.Equal(t, "none", res.SessionDisplay)
		assert.Empty(t, res.Authorized)
	})

	t.Run("Session Not Found", func(t *testing.T) {
		registry := new(infoMockLLMRegistry)
		state := new(infoMockState)
		store := new(infoMockSessionStore)

		registry.On("ListProviders", ctx).Return([]domain.ProviderInfo{}, nil)
		state.On("Model").Return("")
		state.On("CurrentSessionID").Return("missing-sess")
		store.On("Get", "missing-sess").Return(nil, fmt.Errorf("not found"))

		wf := NewInfoWorkflow(registry, state, store)
		res, err := wf.Gather(ctx)

		assert.NoError(t, err)
		assert.Equal(t, "missing-sess (not found)", res.SessionDisplay)
	})

	t.Run("Registry Error", func(t *testing.T) {
		registry := new(infoMockLLMRegistry)
		state := new(infoMockState)
		store := new(infoMockSessionStore)

		registry.On("ListProviders", ctx).Return([]domain.ProviderInfo{}, fmt.Errorf("registry fail"))

		wf := NewInfoWorkflow(registry, state, store)
		_, err := wf.Gather(ctx)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "list providers")
	})
}
