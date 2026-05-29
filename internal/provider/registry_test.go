package provider

import (
	"context"
	"testing"

	"github.com/Cyclone1070/autocmd/internal/domain"
	"github.com/stretchr/testify/assert"
)

type mockProvider struct {
	id string
}

func (m *mockProvider) ID() string                                { return m.id }
func (m *mockProvider) SupportedAuthMethods() []domain.AuthMethod { return nil }
func (m *mockProvider) List() []domain.LLMInfo {
	return []domain.LLMInfo{{ID: m.id + "/" + "model", DisplayName: "Model"}}
}

func (m *mockProvider) GetLLM(_ context.Context, _ *domain.Credential, _ domain.LLMInfo) (domain.LLM, error) {
	return nil, nil
}

type mockStore struct {
	creds map[string]*domain.Credential
}

func (s *mockStore) GetWithFallback(p domain.Provider) (*domain.Credential, error) {
	if s.creds == nil {
		return nil, nil
	}
	return s.creds[p.ID()], nil
}

func TestRegistry(t *testing.T) {
	p := &mockProvider{id: "mock"}
	store := &mockStore{creds: make(map[string]*domain.Credential)}
	pr := NewRegistry(store, p)
	r := NewLLMRegistry(store, pr)

	t.Run("List", func(t *testing.T) {
		providers, err := pr.List(context.Background())
		assert.NoError(t, err)
		assert.Len(t, providers, 1)
		assert.Equal(t, "mock", providers[0].ID)
	})

	t.Run("List returns deterministic sorted provider order", func(t *testing.T) {
		pA := &mockProvider{id: "a-provider"}
		pB := &mockProvider{id: "b-provider"}
		pC := &mockProvider{id: "c-provider"}
		prSorted := NewRegistry(store, pC, pA, pB)

		// Run multiple times to ensure order is stable and sorted.
		for range 20 {
			providers, err := prSorted.List(context.Background())
			assert.NoError(t, err)
			assert.Len(t, providers, 3)
			assert.Equal(t, "a-provider", providers[0].ID)
			assert.Equal(t, "b-provider", providers[1].ID)
			assert.Equal(t, "c-provider", providers[2].ID)
		}
	})

	t.Run("Get", func(t *testing.T) {
		got, ok := pr.Get("mock")
		if !ok || got.ID() != "mock" {
			t.Errorf("expected provider 'mock', got %v", got)
		}
	})

	t.Run("Get with Auto-Resolution", func(t *testing.T) {
		store.creds["mock"] = &domain.Credential{APIKey: "auto"}
		// No cred argument
		_, err := r.Get(context.Background(), "mock/model")
		assert.NoError(t, err)
	})

	t.Run("List with Auto-Resolution", func(t *testing.T) {
		store.creds["mock"] = &domain.Credential{APIKey: "auto", Type: domain.AuthMethodAPIKey}
		// No creds map argument
		llms, err := r.List(context.Background())
		assert.NoError(t, err)
		assert.NotEmpty(t, llms)
	})

	t.Run("List with OAuth Resolution", func(t *testing.T) {
		store.creds["mock"] = &domain.Credential{OAuthToken: "gho_token", Type: "oauth"}
		llms, err := r.List(context.Background())
		assert.NoError(t, err)
		assert.NotEmpty(t, llms)
	})
}
