package domain

// InfoEvent contains gathered data for UI display of configuration and state.
type InfoEvent struct {
	Model          string
	SessionDisplay string
	SessionTokens  int
	ContextWindow  int
	Authorized     []string
}

func (InfoEvent) isUIUpdate() {}

// ModelPickerSnapshot contains the data needed for model selection UI.
type ModelPickerSnapshot struct {
	Models        []LLMInfo
	ActiveModelID string
}

// SessionListEvent contains the data needed for session selection UI.
type SessionListEvent struct {
	Sessions         []SessionSummary
	CurrentSessionID string
}

func (SessionListEvent) isUIUpdate() {}

// AuthProviderSnapshot contains the data needed for the auth management UI.
type AuthProviderSnapshot struct {
	Providers []ProviderSummary
}

// ProviderSummary provides a snapshot of a provider's auth status.
type ProviderSummary struct {
	ID         string
	Authorized bool
}
