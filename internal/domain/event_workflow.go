package domain

// Picker, info-bar, and history workflow→UI events.

import "github.com/cloudwego/eino/schema"

// InfoEvent contains gathered data for UI display of configuration and state.
type InfoEvent struct {
	Model          string
	SessionDisplay string
	Authorized     []string
	SessionTokens  int
	ContextWindow  int
}

func (InfoEvent) isUIUpdate() {}

// ModelListEvent contains the data needed for model selection UI.
type ModelListEvent struct {
	ActiveModelID string
	Models        []LLMInfo
}

func (ModelListEvent) isUIUpdate() {}

// SessionListEvent contains the data needed for session selection UI.
type SessionListEvent struct {
	CurrentSessionID string
	Sessions         []SessionSummary
	WorkingDir       string
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
	AuthMethod string
	Authorized bool
}

// HistoryEvent contains the full conversation history for a session.
type HistoryEvent struct {
	ToolDisplays ToolDisplays
	Messages     []*schema.Message
}

func (HistoryEvent) isUIUpdate() {}
