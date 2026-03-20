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

// ModelListEvent contains the data needed for model selection UI.
type ModelListEvent struct {
	Models        []LLMInfo
	ActiveModelID string
}

func (ModelListEvent) isUIUpdate() {}

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

// HistoryEvent contains the full conversation history for a session.
type HistoryEvent struct {
	Messages     Messages
	ToolDisplays ToolDisplays
}

func (HistoryEvent) isUIUpdate() {}
