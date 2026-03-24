package domain_test

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
)

func TestOAuthMethod(t *testing.T) {
	m := domain.OAuthMethod{
		ID:            "github_oauth",
		Name:          "GitHub Login",
		ClientID:      "test_client",
		DeviceAuthURL: "https://github.com/login/device/code",
		TokenURL:      "https://github.com/login/oauth/access_token",
		Scopes:        []string{"read:user"},
	}

	if m.ID != "github_oauth" {
		t.Errorf("expected ID 'github_oauth', got %s", m.ID)
	}

	var _ domain.AuthMethod = m
}

func TestCredential_OAuth(t *testing.T) {
	c := domain.Credential{
		Type:       "oauth",
		OAuthToken: "gho_secret",
	}

	if c.OAuthToken != "gho_secret" {
		t.Errorf("expected OAuthToken 'gho_secret', got %s", c.OAuthToken)
	}
}
