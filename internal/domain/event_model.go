package domain

// Model picker user actions.

// SelectModelAction is a user intent to switch the current model.
type SelectModelAction struct {
	ID string
}

func (SelectModelAction) isAction() {}
