package domain

// Auth vocabulary: credentials and method descriptors.

// Credential represents a stored or resolved credential for an LLM provider.
type Credential struct {
	Type   string `json:"type"` // e.g., "api_key"
	APIKey string `json:"api_key,omitempty"`

	// Future-proofing for OAuth device flow
	OAuthToken string `json:"oauth_token,omitempty"`
}

// AuthField defines a single input field required for an authentication method.
type AuthField struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder"`
	EnvVar      string `json:"env_var,omitempty"` // Fallback environment variable name
	IsSecret    bool   `json:"is_secret"`
}

// AuthMethod is a marker interface for authentication data descriptors.
type AuthMethod interface {
	IsAuthMethod()
}

// APIKeyAuthMethod represents an authentication method that requires user input of text fields.
type APIKeyAuthMethod struct {
	ID     string
	Name   string
	Fields []AuthField
}

func (APIKeyAuthMethod) IsAuthMethod() {}

// EnvVarAuthMethod represents an authentication method that relies purely on environment variables.
type EnvVarAuthMethod struct {
	ID      string
	Name    string
	EnvVars []string
}

func (EnvVarAuthMethod) IsAuthMethod() {}

// OAuthMethod represents an authentication method that follows the OAuth Device Flow.
type OAuthMethod struct {
	ID            string
	Name          string
	ClientID      string
	DeviceAuthURL string
	TokenURL      string
	Scopes        []string
}

func (OAuthMethod) IsAuthMethod() {}
