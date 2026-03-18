package domain

// InfoResult contains gathered data for UI display of configuration and state.
type InfoResult struct {
	Model          string
	SessionDisplay string
	SessionTokens  int
	ContextWindow  int
	Authorized     []string
}

// ModelPickerResult contains the data needed for model selection UI.
type ModelPickerResult struct {
	Models        []LLMInfo
	ActiveModelID string
}

// SessionPickerResult contains the data needed for session selection UI.
type SessionPickerResult struct {
	Sessions         []SessionSummary
	CurrentSessionID string
}

// AuthWorkflowResult contains the data needed for the auth management UI.
type AuthWorkflowResult struct {
	Providers []ProviderSummary
}

// ProviderSummary provides a snapshot of a provider's auth status.
type ProviderSummary struct {
	ID         string
	Authorized bool
}
