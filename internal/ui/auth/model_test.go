package authui

import (
	"context"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockAuthWorkflow struct {
	mock.Mock
}

func (m *mockAuthWorkflow) Gather(ctx context.Context) (*domain.AuthProviderSnapshot, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.AuthProviderSnapshot), args.Error(1)
}

func (m *mockAuthWorkflow) SetAuth(ctx context.Context, providerID string, cred domain.Credential) error {
	return m.Called(ctx, providerID, cred).Error(0)
}

func (m *mockAuthWorkflow) RemoveAuth(ctx context.Context, providerID string) error {
	return m.Called(ctx, providerID).Error(0)
}

func (m *mockAuthWorkflow) GetProvider(id string) (domain.Provider, bool) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, false
	}
	return args.Get(0).(domain.Provider), args.Bool(1)
}

type mockProvider struct {
	id string
}

func (m *mockProvider) ID() string   { return m.id }
func (m *mockProvider) SupportedAuthMethods() []domain.AuthMethod {
	return []domain.AuthMethod{
		{
			ID:    "api_key",
			Label: "API Key",
			Fields: []domain.AuthField{
				{ID: "key", Label: "Key", Placeholder: "...", IsSecret: true},
			},
		},
	}
}
func (m *mockProvider) ListLLMs() []domain.LLMInfo { return nil }
func (m *mockProvider) GetLLM(ctx context.Context, cred *domain.Credential, modelID string) (domain.LLM, error) {
	return nil, nil
}

func TestAuthUI(t *testing.T) {
	result := &domain.AuthProviderSnapshot{
		Providers: []domain.ProviderSummary{
			{ID: "openai", Authorized: true},
			{ID: "anthropic", Authorized: false},
		},
	}

	t.Run("Initial flow: Load -> List", func(t *testing.T) {
		wf := new(mockAuthWorkflow)
		wf.On("Gather", mock.Anything).Return(result, nil)

		m := NewModel(wf)
		msg := m.Init()()
		m.Update(msg)

		assert.Contains(t, m.View(), "openai")
		wf.AssertExpectations(t)
	})

	t.Run("Delete auth: 'd'", func(t *testing.T) {
		wf := new(mockAuthWorkflow)
		wf.On("RemoveAuth", mock.Anything, "openai").Return(nil)
		wf.On("Gather", mock.Anything).Return(result, nil)

		m := NewModel(wf)
		m.Update(prepareResultMsg{data: result})

		// Press 'd'
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
		assert.NotNil(t, cmd)
		
		msg := cmd() // returns mutationResultMsg{refresh: true}
		_, cmdRefresh := m.Update(msg)
		assert.NotNil(t, cmdRefresh)
		
		msgRefresh := cmdRefresh()
		m.Update(msgRefresh)

		wf.AssertExpectations(t)
	})

	t.Run("Regression: Panic on last field submit", func(t *testing.T) {
		wf := new(mockAuthWorkflow)
		p := &mockProvider{id: "anthropic"}
		wf.On("GetProvider", "anthropic").Return(p, true)
		wf.On("SetAuth", mock.Anything, "anthropic", mock.Anything).Return(nil)

		m := NewModel(wf)
		m.Update(prepareResultMsg{data: result})
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}) // Move to Anthropic
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})                     // Select Provider
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})                     // Select Method

		// Input Key
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("secret")})
		
		// Submit ('enter')
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		
		// CRITICAL: View() should not panic even if index is incremented
		assert.NotPanics(t, func() { m.View() })
	})

	t.Run("Regression: 'q' key should not quit during field collection", func(t *testing.T) {
		wf := new(mockAuthWorkflow)
		p := &mockProvider{id: "anthropic"}
		wf.On("GetProvider", "anthropic").Return(p, true)

		m := NewModel(wf)
		m.Update(prepareResultMsg{data: result})
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}) // Move
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})                     // Select Provider
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})                     // Select Method

		// Press 'q'
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
		
		assert.False(t, m.quitting)
		assert.Contains(t, m.View(), "Key") // Should still be in key collection
	})
}
