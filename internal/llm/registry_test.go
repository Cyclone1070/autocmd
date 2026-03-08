package llm_test

import (
	"context"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/llm"
)

type mockProvider struct {
	id string
}

func (m *mockProvider) ID() string { return m.id }
func (m *mockProvider) SupportedAuthMethods() []domain.AuthMethod { return nil }
func (m *mockProvider) ListLLMs(ctx context.Context, cred *domain.Credential) ([]domain.LLMInfo, error) {
	return []domain.LLMInfo{{ID: "model", DisplayName: "Model"}}, nil
}
func (m *mockProvider) GetLLM(ctx context.Context, cred *domain.Credential, modelID string) (domain.LLM, error) {
	return nil, nil
}

func TestRegistry(t *testing.T) {
	p := &mockProvider{id: "mock"}
	r := llm.NewRegistry(p)

	t.Run("ListProviders", func(t *testing.T) {
		providers := r.ListProviders()
		if len(providers) != 1 || providers[0] != "mock" {
			t.Errorf("expected ['mock'], got %v", providers)
		}
	})

	t.Run("GetProvider", func(t *testing.T) {
		got, ok := r.GetProvider("mock")
		if !ok || got.ID() != "mock" {
			t.Errorf("expected provider 'mock', got %v", got)
		}
	})

	t.Run("GetLLMWithCred", func(t *testing.T) {
		cred := &domain.Credential{APIKey: "key"}
		_, err := r.Get(context.Background(), "mock/model", cred)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ListWithCreds", func(t *testing.T) {
		creds := map[string]*domain.Credential{
			"mock": {APIKey: "key"},
		}
		models, err := r.List(context.Background(), creds)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(models) != 1 || models[0].ID != "mock/model" {
			t.Errorf("expected ['mock/model'], got %v", models)
		}
	})
}
