package domain

// Credential represents a stored or resolved credential for an LLM provider.
type Credential struct {
	Type   string `json:"type"` // e.g., "api_key"
	APIKey string `json:"api_key,omitempty"`

	// Future-proofing for Vertex AI etc.
	Project  string `json:"project,omitempty"`
	Location string `json:"location,omitempty"`
}

// AuthField defines a single input field required for an authentication method.
type AuthField struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder"`
	EnvVar      string `json:"env_var,omitempty"` // Fallback environment variable name
	IsSecret    bool   `json:"is_secret"`
}

// AuthMethod defines a grouping of fields required for a specific authentication type.
type AuthMethod struct {
	ID     string      `json:"id"`
	Label  string      `json:"label"`
	Fields []AuthField `json:"fields"`
}
