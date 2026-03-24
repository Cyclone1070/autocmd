package github

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestProvider(t *testing.T) {
	p := NewProvider()
	
	assert.Equal(t, domain.ProviderGitHub, p.ID())
	
	methods := p.SupportedAuthMethods()
	assert.Len(t, methods, 1) // OAuth only
	
	var foundOAuth bool
	for _, m := range methods {
		if oauth, ok := m.(domain.OAuthMethod); ok {
			assert.Equal(t, "github_oauth", oauth.ID)
			assert.NotEmpty(t, oauth.ClientID)
			foundOAuth = true
		}
	}
	assert.True(t, foundOAuth)
	
	models := p.ListLLMs()
	assert.NotEmpty(t, models)
}
