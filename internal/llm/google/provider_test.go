package google_test

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/llm/google"
)

func TestGoogleProviderAuthSpec(t *testing.T) {
	p := google.NewProvider()
	
	if p.ID() != "google" {
		t.Errorf("expected ID 'google', got %s", p.ID())
	}

	methods := p.SupportedAuthMethods()
	if len(methods) == 0 {
		t.Fatal("expected at least one auth method")
	}

	foundAPIKey := false
	for _, m := range methods {
		if apiKeyMethod, ok := m.(domain.APIKeyAuthMethod); ok {
			if apiKeyMethod.ID == "api_key" {
				foundAPIKey = true
				if len(apiKeyMethod.Fields) == 0 {
					t.Error("expected fields for api_key method")
				}
			}
		}
	}

	if !foundAPIKey {
		t.Error("did not find api_key auth method")
	}
}
