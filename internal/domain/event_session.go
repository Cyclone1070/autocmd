package domain

// Session picker user actions.

// SelectSessionAction is a user intent to choose a session from the list.
type SelectSessionAction struct {
	ID string
}

func (SelectSessionAction) isAction() {}

// CreateSessionAction is a user intent to start a brand new conversation.
type CreateSessionAction struct{}

func (CreateSessionAction) isAction() {}

// RenameSessionAction is a user intent to change a session's title.
type RenameSessionAction struct {
	ID   string
	Name string
}

func (RenameSessionAction) isAction() {}

// DeleteSessionAction is a user intent to remove a session permanently.
type DeleteSessionAction struct {
	ID string
}

func (DeleteSessionAction) isAction() {}
