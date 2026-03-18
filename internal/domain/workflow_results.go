package domain

// SystemSnapshot contains gathered data for UI display of configuration and state.
type SystemSnapshot struct {
	Model          string
	SessionDisplay string
	SessionTokens  int
	ContextWindow  int
	Authorized     []string
}

// ModelPickerSnapshot contains the data needed for model selection UI.
type ModelPickerSnapshot struct {
	Models        []LLMInfo
	ActiveModelID string
}

// SessionPickerSnapshot contains the data needed for session selection UI.
type SessionPickerSnapshot struct {
	Sessions         []SessionSummary
	CurrentSessionID string
}

// AuthProviderSnapshot contains the data needed for the auth management UI.
type AuthProviderSnapshot struct {
	Providers []ProviderSummary
}

// ProviderSummary provides a snapshot of a provider's auth status.
type ProviderSummary struct {
	ID         string
	Authorized bool
}
