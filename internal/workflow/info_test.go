package workflow

import (
	"context"
	"fmt"
	"testing"

	"github.com/Cyclone1070/autocmd/internal/domain"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type infoMockLLMRegistry struct {
	mock.Mock
}

func (m *infoMockLLMRegistry) List(ctx context.Context) ([]domain.ProviderInfo, error) {
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

type infoMockSessionStore struct {
	mock.Mock
}

func (m *infoMockSessionStore) GetSession(id string) (*domain.Session, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Session), args.Error(1)
}

type infoMockLLM struct {
	mock.Mock
}

func (m *infoMockLLM) ID() string          { return m.Called().String(0) }
func (m *infoMockLLM) DisplayName() string { return m.Called().String(0) }
func (m *infoMockLLM) ContextWindow() int  { return m.Called().Int(0) }
func (m *infoMockLLM) Model() model.ToolCallingChatModel {
	return nil
}

func TestInfoWorkflow_Run(t *testing.T) {
	ctx := context.Background()

	t.Run("Full Success Scenario", func(t *testing.T) {
		registry := new(infoMockLLMRegistry)
		state := new(infoMockState)
		store := new(infoMockSessionStore)
		llm := new(infoMockLLM)

		registry.On("List", ctx).Return([]domain.ProviderInfo{
			{ID: domain.ProviderGoogle, Credential: &domain.Credential{Type: domain.AuthMethodEnv}},
			{ID: testFixtureProviderOpenAI, Credential: nil},
		}, nil)

		state.On("Model").Return("google/gemini-pro")

		session := &domain.Session{
			SessionMetadata: domain.SessionMetadata{ID: "sess-123", Name: "Test Session"},
			SessionMessages: domain.SessionMessages{Messages: []*schema.Message{
				{Role: schema.User, Content: "Hello"},
				{
					Role:    schema.Assistant,
					Content: "Hi!",
					ResponseMeta: &schema.ResponseMeta{
						Usage: &schema.TokenUsage{
							TotalTokens: 100,
						},
					},
				},
			}},
		}
		store.On("GetSession", "sess-123").Return(session, nil)

		registry.On("Get", ctx, "google/gemini-pro").Return(llm, nil)
		llm.On("DisplayName").Return("Gemini 1.5 Pro")
		llm.On("ContextWindow").Return(128000)

		wf := NewInfoWorkflow(registry, registry, state, store, "sess-123")
		res, err := wf.gather(ctx)

		assert.NoError(t, err)
		assert.Equal(t, "Gemini 1.5 Pro", res.Model)
		assert.Equal(t, "Test Session", res.SessionDisplay)
		assert.Equal(t, 100, res.SessionTokens)
		assert.Equal(t, 128000, res.ContextWindow)
		assert.ElementsMatch(t, []string{"google (env)"}, res.Authorized)
	})

	t.Run("No Model or Session", func(t *testing.T) {
		registry := new(infoMockLLMRegistry)
		state := new(infoMockState)
		store := new(infoMockSessionStore)

		registry.On("List", ctx).Return([]domain.ProviderInfo{}, nil)
		state.On("Model").Return("")

		wf := NewInfoWorkflow(registry, registry, state, store, "")
		res, err := wf.gather(ctx)

		assert.NoError(t, err)
		assert.Equal(t, "", res.Model)
		assert.Equal(t, "none", res.SessionDisplay)
		assert.Empty(t, res.Authorized)
	})

	t.Run("Session Not Found", func(t *testing.T) {
		registry := new(infoMockLLMRegistry)
		state := new(infoMockState)
		store := new(infoMockSessionStore)

		registry.On("List", ctx).Return([]domain.ProviderInfo{}, nil)
		state.On("Model").Return("")
		store.On("GetSession", "missing-sess").Return(nil, fmt.Errorf("not found"))

		wf := NewInfoWorkflow(registry, registry, state, store, "missing-sess")
		res, err := wf.gather(ctx)

		assert.NoError(t, err)
		assert.Equal(t, "missing-sess (not found)", res.SessionDisplay)
	})

	t.Run("Registry Error", func(t *testing.T) {
		registry := new(infoMockLLMRegistry)
		state := new(infoMockState)
		store := new(infoMockSessionStore)

		registry.On("List", ctx).Return([]domain.ProviderInfo{}, fmt.Errorf("registry fail"))

		wf := NewInfoWorkflow(registry, registry, state, store, "")
		_, err := wf.gather(ctx)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "list providers")
	})
}
