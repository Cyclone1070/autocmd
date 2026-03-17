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
