package provider

import (
	"testing"

	"github.com/Cyclone1070/autocmd/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestOpenCodeProvider(t *testing.T) {
	p := NewOpenCodeProvider(nil)

	assert.Equal(t, domain.ProviderOpenCode, p.ID())

	methods := p.SupportedAuthMethods()
	assert.NotEmpty(t, methods)

	foundAPIKey := false
	foundEnv := false
	for _, m := range methods {
		if apiKeyMethod, ok := m.(domain.APIKeyAuthMethod); ok {
			if apiKeyMethod.ID == domain.AuthMethodAPIKey {
				foundAPIKey = true
				assert.NotEmpty(t, apiKeyMethod.Fields)
				for _, f := range apiKeyMethod.Fields {
					if f.ID == domain.AuthFieldAPIKey {
						assert.Equal(t, "OPENCODE_API_KEY", f.EnvVar)
					}
				}
			}
		}
		if envMethod, ok := m.(domain.EnvVarAuthMethod); ok {
			if envMethod.ID == domain.AuthMethodEnv {
				foundEnv = true
				assert.Contains(t, envMethod.EnvVars, "OPENCODE_API_KEY")
			}
		}
	}

	assert.True(t, foundAPIKey, "should have api key auth method")
	assert.True(t, foundEnv, "should have env var auth method")
}
