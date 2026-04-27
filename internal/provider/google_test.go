package provider

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestGoogleProviderAuthSpec(t *testing.T) {
	p := NewGoogleProvider(nil)

	assert.Equal(t, "google", p.ID())

	methods := p.SupportedAuthMethods()
	assert.NotEmpty(t, methods)

	foundAPIKey := false
	for _, m := range methods {
		if apiKeyMethod, ok := m.(domain.APIKeyAuthMethod); ok {
			if apiKeyMethod.ID == "api_key" {
				foundAPIKey = true
				assert.NotEmpty(t, apiKeyMethod.Fields)
			}
		}
	}

	assert.True(t, foundAPIKey)
}
