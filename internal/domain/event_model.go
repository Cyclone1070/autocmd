package domain

// Model picker user actions.

// SelectModelAction is a user intent to switch the current model.
type SelectModelAction struct {
	ID string
}

func (SelectModelAction) isAction() {}

// ModelListEvent contains the data needed for model selection UI.
type ModelListEvent struct {
	ActiveModelID string
	Models        []LLMInfo
}

func (ModelListEvent) isUIUpdate() {}
