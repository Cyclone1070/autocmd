package domain

// InfoEvent contains gathered data for UI display of configuration and state.
type InfoEvent struct {
	Model          string
	SessionDisplay string
	Authorized     []string
	SessionTokens  int
	ContextWindow  int
}

func (InfoEvent) isUIUpdate() {}
