package google_test

import (
	"testing"

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
		if m.ID == "api_key" {
			foundAPIKey = true
			if len(m.Fields) == 0 {
				t.Error("expected fields for api_key method")
			}
		}
	}

	if !foundAPIKey {
		t.Error("did not find api_key auth method")
	}
}
