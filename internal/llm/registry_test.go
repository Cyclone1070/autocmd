package llm

import (
	"context"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
)

type mockProvider struct {
	id string
}

func (m *mockProvider) ID() string                                     { return m.id }
func (m *mockProvider) SupportedAuthMethods() []domain.AuthMethod      { return nil }
func (m *mockProvider) ListLLMs() []domain.LLMInfo {
	return []domain.LLMInfo{{ID: "model", DisplayName: "Model"}}
}
func (m *mockProvider) GetLLM(ctx context.Context, cred *domain.Credential, modelID string) (domain.LLM, error) {
	return nil, nil
}

type mockStore struct {
	creds map[string]*domain.Credential
}

func (s *mockStore) GetWithFallback(p domain.Provider) (*domain.Credential, error) {
	return s.creds[p.ID()], nil
}

func TestRegistry(t *testing.T) {
	p := &mockProvider{id: "mock"}
	store := &mockStore{creds: make(map[string]*domain.Credential)}
	r := NewRegistry(store, p)

	t.Run("ListProviders", func(t *testing.T) {
		providers, err := r.ListProviders(context.Background())
		assert.NoError(t, err)
		assert.Len(t, providers, 1)
		assert.Equal(t, "mock", providers[0].ID)
	})

	t.Run("ListProviders returns deterministic sorted provider order", func(t *testing.T) {
		pA := &mockProvider{id: "a-provider"}
		pB := &mockProvider{id: "b-provider"}
		pC := &mockProvider{id: "c-provider"}
		rSorted := NewRegistry(store, pC, pA, pB)

		// Run multiple times to ensure order is stable and sorted.
		for i := 0; i < 20; i++ {
			providers, err := rSorted.ListProviders(context.Background())
			assert.NoError(t, err)
			assert.Len(t, providers, 3)
			assert.Equal(t, "a-provider", providers[0].ID)
			assert.Equal(t, "b-provider", providers[1].ID)
			assert.Equal(t, "c-provider", providers[2].ID)
		}
	})

	t.Run("GetProvider", func(t *testing.T) {
		got, ok := r.GetProvider("mock")
		if !ok || got.ID() != "mock" {
			t.Errorf("expected provider 'mock', got %v", got)
		}
	})

	t.Run("Get with Direct Credential - REMOVED", func(t *testing.T) {
		// This test is no longer valid as we removed the explicit cred argument
		// We'll rely on the auto-resolution tests
	})

	t.Run("Get with Auto-Resolution", func(t *testing.T) {
		store.creds["mock"] = &domain.Credential{APIKey: "auto"}
		// No cred argument
		_, err := r.Get(context.Background(), "mock/model")
		assert.NoError(t, err)
	})

	t.Run("List with Auto-Resolution", func(t *testing.T) {
		store.creds["mock"] = &domain.Credential{APIKey: "auto", Type: "api_key"}
		// No creds map argument
		llms, err := r.List(context.Background())
		assert.NoError(t, err)
		assert.NotEmpty(t, llms)
	})

	t.Run("ListWithCreds - REMOVED", func(t *testing.T) {
		// This test is no longer valid as we removed the explicit creds argument
	})
}
