package workflow

import (
	"context"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockModelLLMRegistry struct {
	mock.Mock
}

func (m *mockModelLLMRegistry) List(ctx context.Context) ([]domain.LLMInfo, error) {
	args := m.Called(ctx)
	return args.Get(0).([]domain.LLMInfo), args.Error(1)
}

type mockModelState struct {
	mock.Mock
}

func (m *mockModelState) Model() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockModelState) SetModel(id string) {
	m.Called(id)
}

func (m *mockModelState) Save() error {
	args := m.Called()
	return args.Error(0)
}

func TestModelPickerWorkflow_Run(t *testing.T) {
	ctx := context.Background()
	registry := new(mockModelLLMRegistry)
	state := new(mockModelState)

	models := []domain.LLMInfo{
		{ID: "google/gemini-pro", DisplayName: "Gemini Pro"},
		{ID: "openai/gpt-4", DisplayName: "GPT-4"},
	}

	registry.On("List", ctx).Return(models, nil)
	state.On("Model").Return("google/gemini-pro")

	wf := NewModelPickerWorkflow(registry, state)
	res, err := wf.Run(ctx)

	assert.NoError(t, err)
	assert.Equal(t, "google/gemini-pro", res.ActiveModelID)
	assert.Len(t, res.Models, 2)
}

func TestModelPickerWorkflow_Select(t *testing.T) {
	ctx := context.Background()
	registry := new(mockModelLLMRegistry)
	state := new(mockModelState)

	state.On("SetModel", "openai/gpt-4").Return()
	state.On("Save").Return(nil)

	wf := NewModelPickerWorkflow(registry, state)
	err := wf.Select(ctx, "openai/gpt-4")

	assert.NoError(t, err)
	state.AssertExpectations(t)
}
