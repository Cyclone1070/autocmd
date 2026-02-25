package domain

// Credential represents a stored or resolved credential for an LLM provider.
type Credential struct {
	Type   string `json:"type"` // e.g., "api_key"
	APIKey string `json:"api_key,omitempty"`

	// Future-proofing for Vertex AI etc.
	Project  string `json:"project,omitempty"`
	Location string `json:"location,omitempty"`
}
