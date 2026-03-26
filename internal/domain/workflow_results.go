package domain

import "github.com/cloudwego/eino/schema"

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

// AuthProviderListEvent contains the data needed for the auth management UI.
type AuthProviderListEvent struct {
	Providers []ProviderSummary
}

func (AuthProviderListEvent) isUIUpdate() {}

// ProviderSummary provides a snapshot of a provider's auth status.
type ProviderSummary struct {
	ID         string
	Authorized bool
	AuthMethod string
}

// HistoryEvent contains the full conversation history for a session.
type HistoryEvent struct {
	Messages     []*schema.Message
	ToolDisplays ToolDisplays
}

func (HistoryEvent) isUIUpdate() {}
