package domain

// User actions: Action marker and StopAction (other actions live in event_*.go).

// Action is the interface for all intents flowing from UI to Workflow.
type Action interface {
	isAction()
}

// StopAction is a user intent to cancel the current workflow.
type StopAction struct{}

func (StopAction) isAction() {}
